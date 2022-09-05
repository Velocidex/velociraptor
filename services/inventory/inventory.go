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
	"path"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type githubReleasesAPI struct {
	Assets []githubAssets `json:"assets"`
}

type githubAssets struct {
	Name               string `json:"name"`
	BrowserDownloadUrl string `json:"browser_download_url"`
}

type InventoryService struct {
	mu       sync.Mutex
	binaries *artifacts_proto.ThirdParty
	Client   HTTPClient
	Clock    utils.Clock
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

func (self *InventoryService) ProbeToolInfo(name string) (*artifacts_proto.Tool, error) {
	for _, tool := range self.Get().Tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return nil, errors.New("Not Found")
}

// Gets the tool information from the inventory. If the tool is not
// already downloaded, we download it and update the hashes.
func (self *InventoryService) GetToolInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool string) (*artifacts_proto.Tool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	for _, item := range self.binaries.Tools {
		if item.Name == tool {
			// Currently we require to know all tool's
			// hashes. If the hash is missing then the
			// tool is not tracked. We have to materialize
			// it in order to track it.
			if item.Hash == "" {
				// Try to download the item.
				err := self.materializeTool(ctx, config_obj, item)
				if err != nil {
					return nil, err
				}
			}
			return proto.Clone(item).(*artifacts_proto.Tool), nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Tool %v not declared in inventory.", tool))
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
			return errors.Wrap(
				err, "While resolving github release "+tool.GithubProject)
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

	// All tools are stored at the global public directory which is
	// mapped to a http static handler. The downloaded URL is
	// regardless of org - however each org has a different download
	// name. We need to write the tool on the root org's public
	// directory.
	org_manager, err := services.GetOrgManager()
	root_org_config, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(root_org_config)
	if file_store_factory == nil {
		return errors.New("No filestore configured")
	}

	// All tools are written to the root org's public directory since
	// this is the only one mapped for external access. File names
	// should never clash because the names are derived from a hash
	// mixed with org id and filename so should be unique to each
	// org. Therefore we use the root orgs file store but get a path
	// manager specific to each org.
	path_manager := paths.NewInventoryPathManager(org_config_obj, tool)
	fd, err := file_store_factory.WriteFile(path_manager.Path())
	if err != nil {
		return err
	}
	defer fd.Close()

	err = fd.Truncate()
	if err != nil {
		return err
	}

	logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
	logger.Info("Downloading tool <green>%v</> FROM <red>%v</>", tool.Name,
		tool.Url)
	request, err := http.NewRequestWithContext(ctx, "GET", tool.Url, nil)
	if err != nil {
		return err
	}
	res, err := self.Client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// If the download failed, we can not store this tool.
	if res.StatusCode != 200 {
		return errors.New(fmt.Sprintf("Unable to download file from %v: %v",
			tool.Url, res.Status))
	}
	sha_sum := sha256.New()

	_, err = utils.Copy(ctx, fd, io.TeeReader(res.Body, sha_sum))
	if err == nil {
		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	if tool.ServeLocally {
		if org_config_obj.Client == nil || len(org_config_obj.Client.ServerUrls) == 0 {
			return errors.New("No server URLs configured!")
		}
		tool.ServeUrl = org_config_obj.Client.ServerUrls[0] + "public/" + tool.FilestorePath

	} else {
		tool.ServeUrl = tool.Url
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

func (self *InventoryService) AddTool(config_obj *config_proto.Config,
	tool_request *artifacts_proto.Tool, opts services.ToolOptions) error {
	if opts.Upgrade {
		existing_tool, err := self.ProbeToolInfo(tool_request.Name)
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

	if tool.ServeLocally && config_obj.Client == nil {
		tool.ServeLocally = false
	}

	if tool.ServeLocally {
		if config_obj.Client == nil || len(config_obj.Client.ServerUrls) == 0 {
			return errors.New("No server URLs configured!")
		}
		tool.ServeUrl = config_obj.Client.ServerUrls[0] + "public/" + tool.FilestorePath
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
		if item.Name == tool.Name {
			found = true
			self.binaries.Tools[i] = tool
			break
		}
	}

	if !found {
		self.binaries.Tools = append(self.binaries.Tools, tool)
	}

	self.binaries.Version = uint64(self.Clock.Now().UnixNano())

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(config_obj, paths.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) LoadFromFile(config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

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

	default_client, err := networking.GetDefaultHTTPClient(config_obj.Client, "")
	if err != nil {
		return nil, err
	}

	inventory_service := &InventoryService{
		Clock:    utils.RealClock{},
		binaries: &artifacts_proto.ThirdParty{},
		// Use the VQL http client so it can accept the same certs.
		Client: default_client,
	}
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notifier, err := services.GetNotifier(config_obj)
	if err != nil {
		return nil, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inventory_service.Close()

		for {
			// Watch for notifications that the inventory is changed.
			notification, cancel := notifier.ListenForNotification(
				"Server.Internal.Inventory")

			select {
			case <-ctx.Done():
				return

			case <-notification:
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}

			case <-time.After(600 * time.Second):
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}
			}

			cancel()
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
