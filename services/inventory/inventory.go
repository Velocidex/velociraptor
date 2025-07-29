/*

  Velociraptor maintains third party files in an inventory. This
  service manages this inventory.

  Tool definitions can be added using AddTool() - this only writes the
  definition to the internal datastore without materializing the tool.

  The tool definition is divided into user accessible and system
  accessible parts. The user specifies fields like:

  - name
  - url (upstream url)
  - github_project

  The system takes these and generates tracking information such as

  - hash - the expected hash of the file - required!
  - serve_url  - where we get users to download the file from.

  Tools may be added to the inventory service without being tracked -
  in that case they will not have a valid hash, serve_url etc. When we
  attempt to use the tool with GetToolInfo() they will be materialized
  and tracked automatically.

  If AddTool() specifies the hash and serve_url then we assume the
  tool is tracked and use that. This allows the admin to force a
  specific tool to be used, by e.g. uploading it to the public
  directory manually and adding the expected hash, but not providing a
  URL. This is what the `velociraptor tools upload` command and the
*/

package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

const ALL_VERSIONS = ""

var (
	inventoryTotalLoad = promauto.NewCounter(prometheus.CounterOpts{
		Name: "inventory_service_total_file_load",
		Help: "Total number of times we synced from the filestore.",
	})
)

type githubReleasesAPI struct {
	Assets []githubAssets `json:"assets"`
}

type githubAssets struct {
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"browser_download_url"`
}

type InventoryService struct {
	mu sync.Mutex

	// An index of all tools keyed by name, version. These tools are
	// possibly materialized or updated by the user and contain more
	// information than the original definition within the artifact.
	binaries *artifacts_proto.ThirdParty

	// An index of all original definitions keyed by tool name and
	// version. It is possible to reset the definitions in `binaries`
	// above with one of these original definitions.
	versions map[string][]*artifacts_proto.Tool

	// A HTTPClient that is used to download tools automatically.
	Client networking.HTTPClient

	// The parent is the inventory service of the root org. The root
	// org maintain the parent's repository and takes the default
	// settings.
	parent services.Inventory
}

func (self *InventoryService) Close() {}

func (self *InventoryService) Get() *artifacts_proto.ThirdParty {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.binaries).(*artifacts_proto.ThirdParty)
}

func (self *InventoryService) ClearForTests() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.binaries = &artifacts_proto.ThirdParty{}
}

func (self *InventoryService) ProbeToolInfo(
	ctx context.Context, config_obj *config_proto.Config,
	name, version string) (*artifacts_proto.Tool, error) {

	var match *artifacts_proto.Tool
	for _, tool := range self.Get().Tools {
		// If version is specified we look for the exact tool version
		if version != "" {
			if tool.Name == name && tool.Version == version {
				return self.AddAllVersions(ctx, config_obj, tool, version), nil
			}
			continue
		}

		if tool.Name != name {
			continue
		}

		// Otherwise get the latest version available
		if match == nil {
			match = tool
			continue
		}

		if utils.CompareVersions(tool.Name, match.Version, tool.Version) < 0 {
			match = tool
		}
	}

	if match != nil {
		return self.AddAllVersions(ctx, config_obj, match, ALL_VERSIONS), nil
	}

	if self.parent != nil {
		tool, err := self.parent.ProbeToolInfo(ctx, config_obj, name, version)
		if err == nil {
			// Add all the parent's versions into our own repository.
			for _, v := range tool.Versions {
				err := self.AddTool(ctx, config_obj, v, services.ToolOptions{
					ArtifactDefinition: true,
				})
				if err != nil {
					return nil, err
				}
			}

			// Return the first version that matched
			for _, tool := range tool.Versions {
				// If version is specified we look for the exact tool version
				if version != "" {
					if tool.Name == name && tool.Version == version {
						return self.AddAllVersions(
							ctx, config_obj, tool, version), nil
					}
					continue
				}

				if tool.Name == name {
					return self.AddAllVersions(
						ctx, config_obj, tool, ALL_VERSIONS), nil
				}
			}
		}
	}

	return nil, errors.New("Not Found")
}

// Enrich the tool definition with all known versions of this tool.
func (self *InventoryService) AddAllVersions(
	ctx context.Context, config_obj *config_proto.Config,
	tool *artifacts_proto.Tool, required_version string) *artifacts_proto.Tool {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.addAllVersions(ctx, config_obj, tool, required_version)
}

// The same tool may be defined in multiple artifacts and these
// definitions may be different. This function collects all compatible
// definitions into the Tool protobuf. This gives us a list of all
// definitions of the tool.
//
// Compatible definitions are those with the same name and version.
// The user may select one of these definitions to be used by all
// artifacts.
func (self *InventoryService) addAllVersions(
	ctx context.Context, config_obj *config_proto.Config,
	tool *artifacts_proto.Tool, required_version string) *artifacts_proto.Tool {
	result := proto.Clone(tool).(*artifacts_proto.Tool)

	versions, _ := self.versions[tool.Name]
	result.Versions = nil

	for _, v := range versions {
		if required_version != "" && required_version != v.Version {
			continue
		}

		result.Versions = append(result.Versions, v)
	}

	// Merge the parent's versions as well.
	if self.parent != nil {
		parent_tool, err := self.parent.ProbeToolInfo(ctx, config_obj, tool.Name, "")
		if err == nil {
			result.Versions = append(result.Versions, parent_tool.Versions...)
		}
	}

	return result
}

// Gets the tool information from the inventory. If the tool is not
// already downloaded, we download it and update the hashes.
func (self *InventoryService) GetToolInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool, version string) (*artifacts_proto.Tool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// If a version is not specified, we need to sort the tools by
	// semantic version so we get the latest version available.
	var match *artifacts_proto.Tool
	for _, item := range self.binaries.Tools {
		if item.Name != tool {
			continue
		}

		if version != "" && version == item.Version {
			match = item
			break
		}

		// Look for the largest version available
		if match == nil {
			match = item
			continue
		}

		if utils.CompareVersions(item.Name, match.Version, item.Version) < 0 {
			match = item
		}
	}

	if match != nil {
		// Currently we require to know all tool's hashes. If the hash
		// is missing then the tool is not tracked. We have to
		// materialize it in order to track it.
		if match.Hash == "" {
			// Try to download the item.
			err := self.materializeTool(ctx, config_obj, match)
			if err != nil {
				return nil, err
			}
		}

		// Already holding the mutex here - call the lock free
		// version.
		match = self.addAllVersions(ctx, config_obj, match, version)

		return decorateServeUrls(config_obj, match)
	}

	return nil, fmt.Errorf("Tool %v not declared in inventory.", tool)
}

// Actually download and resolve the tool and make sure it is
// available. If successful this function updates the tool's datastore
// representation to track it (in particular the hash). Subsequent
// calls to this function will just retrieve those fields directly.
func (self *InventoryService) materializeTool(
	ctx context.Context,
	org_config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) error {

	if self.Client == nil {
		return errors.New("Client not configured")
	}

	// If we are downloading from github we have to resolve and
	// verify the binary URL now.
	if tool.GithubProject != "" {
		var err error
		tool.Url, err = getGithubRelease(ctx, self.Client, org_config_obj, tool)
		if err != nil {
			return fmt.Errorf(
				"While resolving github release %v: %w ",
				tool.GithubProject, err)
		}

		// Set the filename to something sensible so it is always valid.
		if tool.Filename == "" {
			if tool.Url != "" {
				tool.Filename = path.Base(tool.Url)
			} else {
				tool.Filename = path.Base(tool.ServeUrl)
			}
		}
	}

	// We have no idea where the file is.
	if tool.Url == "" {
		return fmt.Errorf("Tool %v has no url defined - upload it manually.",
			tool.Name)
	}

	// All tools are written to the root org's public directory since
	// this is the only one mapped for external access. File names
	// should never clash because the names are derived from a hash
	// mixed with org id and filename so should be unique to each
	// org. Therefore we use the root orgs file store but get a path
	// manager specific to each org.
	path_manager := paths.NewInventoryPathManager(org_config_obj, tool)
	pathspec, file_store_factory, err := path_manager.Path()
	if err != nil {
		return err
	}

	fd, err := file_store_factory.WriteFile(pathspec)
	if err != nil {
		return err
	}
	defer fd.Close()

	err = fd.Truncate()
	if err != nil {
		return err
	}

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
	logger.Info("Downloading tool <green>%v</> FROM <cyan>%v</>", tool.Name,
		tool.Url)
	request, err := http.NewRequestWithContext(ctx, "GET", tool.Url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", constants.USER_AGENT)
	res, err := self.Client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// If the download failed, we can not store this tool.
	if res.StatusCode != 200 {
		return fmt.Errorf("Unable to download file from %v: %v",
			tool.Url, res.Status)
	}
	sha_sum := sha256.New()

	_, err = utils.Copy(ctx, fd, io.TeeReader(res.Body, sha_sum))
	if err == nil {
		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	if tool.ExpectedHash != "" && !strings.EqualFold(tool.ExpectedHash, tool.Hash) {
		err := fmt.Errorf(
			"Downloaded tool hash of %v does not match the expected hash of %v\n",
			tool.Hash, tool.ExpectedHash)

		// Record the invalid hash so the user can opt to trust it.
		tool.InvalidHash = tool.Hash
		tool.Hash = ""
		return err
	}
	tool.InvalidHash = ""

	if tool.ServeLocally {
		tool.ServeUrls, err = getPublicURLs(org_config_obj, "public/"+tool.FilestorePath)
		if err != nil {
			return err
		}
		if len(tool.ServeUrls) > 0 {
			tool.ServeUrl = tool.ServeUrls[0]
		}

	} else {
		tool.ServeUrl = tool.Url
		tool.ServeUrls = []string{tool.ServeUrl}
	}

	db, err := datastore.GetDB(org_config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(org_config_obj, paths.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) RemoveTool(
	config_obj *config_proto.Config, tool_name string) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	tools := []*artifacts_proto.Tool{}
	for _, item := range self.binaries.Tools {
		if item.Name != tool_name {
			tools = append(tools, item)
		}
	}

	self.binaries.Tools = tools

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(config_obj, paths.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) UpdateVersion(tool_request *artifacts_proto.Tool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Update the list of versions for this tool, replacing existing
	// definitions.
	versions, _ := self.versions[tool_request.Name]
	version_known := false
	for idx, v := range versions {
		if v.Artifact == tool_request.Artifact {
			versions[idx] = tool_request
			version_known = true
			break
		}
	}

	if !version_known {
		versions = append(versions, proto.Clone(tool_request).(*artifacts_proto.Tool))
	}
	self.versions[tool_request.Name] = versions
}

// This gets called by the repository for each artifact loaded to
// inform us about any tools it contains. The InventoryService looks
// at its current definition for the tool in the inventory to see if
// it needs to upgrade the definition or add a new entry to the
// inventory automatically.
func (self *InventoryService) AddTool(
	ctx context.Context, config_obj *config_proto.Config,
	tool_request *artifacts_proto.Tool, opts services.ToolOptions) (err error) {

	// Clear out the system managed fields.
	tool_request.Versions = nil
	tool_request.ServeUrl = ""
	tool_request.InvalidHash = ""

	// Keep a reference to all known versions of this tool. We keep
	// the clean definitions from the artifact together, so we can
	// always reset back to them.
	if opts.ArtifactDefinition {
		self.UpdateVersion(tool_request)
	}

	if opts.Upgrade {
		existing_tool, err := self.ProbeToolInfo(
			ctx, config_obj, tool_request.Name, tool_request.Version)
		if err == nil {
			// Ignore the request if the existing
			// definition is better than the new one.
			if isDefinitionBetter(existing_tool, tool_request) {
				return nil
			}
		}
	}

	if opts.AdminOverride {
		tool_request.AdminOverride = true
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// Obfuscate the public directory path.
	// Make a copy to work on.
	tool := proto.Clone(tool_request).(*artifacts_proto.Tool)
	tool.FilestorePath = paths.ObfuscateName(config_obj, tool.Name)

	// No client config so we dont know any server urls - therefore we
	// can not serve locally at all.
	if tool.ServeLocally && config_obj.Client == nil {
		tool.ServeLocally = false
	}

	if tool.ServeLocally {
		tool.ServeUrls, err = getPublicURLs(config_obj, "public/"+tool.FilestorePath)
		if err != nil {
			return err
		}
		if len(tool.ServeUrls) > 0 {
			tool.ServeUrl = tool.ServeUrls[0]
		}
	} else {
		// If we dont serve the tool, the clients will directly get
		// the tool from its upstream URL.
		tool.ServeUrl = tool.Url
		tool.ServeUrls = []string{tool.ServeUrl}
	}

	// Set the filename to something sensible so it is always valid.
	if tool.Filename == "" {
		if tool.Url != "" {
			tool.Filename = path.Base(tool.Url)
		}
	}

	// Replace the tool in the inventory.
	found := false
	for i, item := range self.binaries.Tools {
		if item.Name == tool.Name &&
			item.Version == tool.Version {
			found = true
			self.binaries.Tools[i] = tool
			break
		}
	}

	if !found {
		self.binaries.Tools = append(self.binaries.Tools, tool)
	}

	self.binaries.Version = uint64(utils.GetTime().Now().UnixNano())

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		// If the datastore is not available this is not an error - we
		// just do not write the inventory to storage. This happens
		// when we a client starts the inventory service instead of
		// DummyService.
		return nil
	}
	err = db.SetSubject(config_obj, paths.ThirdPartyInventory, self.binaries)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Warn("Unable to store inventory - will run with an in memory one.")
	}
	return nil
}

func (self *InventoryService) LoadFromFile(config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	inventoryTotalLoad.Inc()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("InventoryService: Reloading inventory from file for org %v",
		config_obj.OrgId)

	inventory := &artifacts_proto.ThirdParty{}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	err = db.GetSubject(config_obj, paths.ThirdPartyInventory, inventory)

	// Ignore errors from reading the inventory file - it might be
	// missing or corrupt but this is not an error - just try again later.
	_ = err

	self.binaries = inventory

	return nil
}

func NewInventoryService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Inventory, error) {

	if config_obj.Datastore == nil {
		return NewInventoryDummyService(ctx, wg, config_obj)
	}

	scope := vql_subsystem.MakeScope()
	default_client, err := networking.GetDefaultHTTPClient(
		ctx, config_obj.Client, scope, "", networking.EmptyCookieJar)
	if err != nil {
		return nil, err
	}

	inventory_service := &InventoryService{
		binaries: &artifacts_proto.ThirdParty{},
		versions: make(map[string][]*artifacts_proto.Tool),
		// Use the VQL http client so it can accept the same certs.
		Client: default_client,
	}
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	if config_obj.Security != nil &&
		config_obj.Security.DisableInventoryServiceExternalAccess {
		inventory_service.Client = DummyHTTPClient{}
	}

	// If we are not the root inventory we need to delegate any
	// unknown tools to the root inventory.
	if !utils.IsRootOrg(config_obj.OrgId) {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, err
		}

		root_org_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
		if err != nil {
			return nil, err
		}

		root_inventory_service, err := services.GetInventory(root_org_config)
		if err != nil {
			return nil, err
		}

		inventory_service.parent = root_inventory_service
	}

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return nil, err
	}

	// Reload the inventory_service when another server wakes us up.
	row_chan, cancel := journal.Watch(ctx,
		"Server.Internal.Inventory", "InventoryService")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inventory_service.Close()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case _, ok := <-row_chan:
				if !ok {
					return
				}

				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}

			case <-time.After(utils.Jitter(600 * time.Second)):
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}
			}
		}
	}()

	logger.Info("<green>Starting</> Inventory Service for %v",
		services.GetOrgName(config_obj))

	// If we fail to load from the file start from a new empty
	// inventory.
	_ = inventory_service.LoadFromFile(config_obj)

	return inventory_service, nil
}

func isDefinitionBetter(old, new *artifacts_proto.Tool) bool {
	// Admin wants to set the tool, always honor it.
	if new.AdminOverride {
		return false
	}

	// The admin is always right - never override a tool set by
	// the admin.
	if old.AdminOverride {
		return true
	}

	// We really do not know where to fetch the old tool from
	// anyway - the new tool must be better.
	if old.Url == "" && old.GithubProject == "" && old.ServeUrl == "" {
		return false
	}

	// We prefer to keep the old tool.
	return true
}

func decorateServeUrls(config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) (res *artifacts_proto.Tool, err error) {

	tool = proto.Clone(tool).(*artifacts_proto.Tool)

	if tool.ServeLocally {
		tool.ServeUrls, err = getPublicURLs(config_obj, "public/"+tool.FilestorePath)
		if err != nil {
			return nil, err
		}
		if len(tool.ServeUrls) > 0 {
			tool.ServeUrl = tool.ServeUrls[0]
		}

	} else {
		tool.ServeUrl = tool.Url
		tool.ServeUrls = []string{tool.ServeUrl}
	}

	return tool, nil
}

// Calculates the URL of the /public/ directory from the config file.
func getPublicURLs(config_obj *config_proto.Config, path string) (res []string, err error) {
	if config_obj.Client == nil || len(config_obj.Client.ServerUrls) == 0 {
		return nil, fmt.Errorf("%w: No server URLs configured!", utils.InvalidConfigError)
	}

	for _, server_url := range config_obj.Client.ServerUrls {
		parsed_url, err := url.Parse(server_url)
		if err != nil {
			return nil, fmt.Errorf("%w: %w!", utils.InvalidConfigError, err)
		}

		if parsed_url.Scheme == "wss" {
			parsed_url.Scheme = "https"
		}

		parsed_url.Path += path

		res = append(res, parsed_url.String())
	}

	return res, err
}
