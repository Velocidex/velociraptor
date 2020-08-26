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
	"net"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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
	db       datastore.DataStore
	Clock    utils.Clock
}

func (self *InventoryService) Close() {}

func (self *InventoryService) Get() *artifacts_proto.ThirdParty {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.binaries).(*artifacts_proto.ThirdParty)
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
	config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) error {

	// If we are downloading from github we have to resolve and
	// verify the binary URL now.
	if tool.GithubProject != "" {
		var err error
		tool.Url, err = getGithubRelease(ctx, self.Client, config_obj, tool)
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

	file_store_factory := file_store.GetFileStore(config_obj)
	if file_store_factory == nil {
		return errors.New("No filestore configured")
	}

	path_manager := paths.NewInventoryPathManager(config_obj, tool)
	fd, err := file_store_factory.WriteFile(path_manager.Path())
	if err != nil {
		return err
	}
	defer fd.Close()

	err = fd.Truncate()
	if err != nil {
		return err
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
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
		if config_obj.Client == nil || len(config_obj.Client.ServerUrls) == 0 {
			return errors.New("No server URLs configured!")
		}
		tool.ServeUrl = config_obj.Client.ServerUrls[0] + "public/" + tool.FilestorePath

	} else {
		tool.ServeUrl = tool.Url
	}

	return self.db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
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

	return self.db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) AddTool(config_obj *config_proto.Config,
	tool_request *artifacts_proto.Tool) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// Obfuscate the public directory path.
	// Make a copy to work on.
	tool := *tool_request
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
			self.binaries.Tools[i] = &tool
			break
		}
	}

	if !found {
		self.binaries.Tools = append(self.binaries.Tools, &tool)
	}

	self.binaries.Version = uint64(self.Clock.Now().UnixNano())

	return self.db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) LoadFromFile(config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	inventory := &artifacts_proto.ThirdParty{}
	err := self.db.GetSubject(config_obj, constants.ThirdPartyInventory, inventory)

	// Ignore errors from reading the inventory file - it might be
	// missing or corrupt but this is not an error - just try again later.
	_ = err

	self.binaries = inventory

	return nil
}

func StartInventoryService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Datastore == nil {
		return StartInventoryDummyService(ctx, wg, config_obj)
	}

	inventory_service := &InventoryService{
		Clock:    utils.RealClock{},
		binaries: &artifacts_proto.ThirdParty{},
		db:       datastore.NewTestDataStore(),
		Client: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   300 * time.Second,
					KeepAlive: 300 * time.Second,
					DualStack: true,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       300 * time.Second,
				TLSHandshakeTimeout:   100 * time.Second,
				ExpectContinueTimeout: 10 * time.Second,
				ResponseHeaderTimeout: 100 * time.Second,
			},
		},
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	inventory_service.db = db

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notification, cancel := services.GetNotifier().ListenForNotification(
		constants.ThirdPartyInventory)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer inventory_service.Close()

		for {
			select {
			case <-ctx.Done():
				return

			case <-notification:
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}

			case <-time.After(time.Second):
				err = inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: %v", err)
				}
			}

			cancel()
			notifier := services.GetNotifier()
			if notifier == nil {
				return
			}
			notification, cancel = notifier.ListenForNotification(
				constants.ThirdPartyInventory)
		}
	}()

	logger.Info("<green>Starting</> Inventory Service")

	services.RegisterInventory(inventory_service)
	_ = inventory_service.LoadFromFile(config_obj)

	return nil
}
