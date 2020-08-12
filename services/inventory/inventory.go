/*

  Velociraptor maintains third party files in an inventory. This
  service manages this inventory.

*/

package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"regexp"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
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

func (self *InventoryService) Get() *artifacts_proto.ThirdParty {
	self.mu.Lock()
	defer self.mu.Unlock()

	return proto.Clone(self.binaries).(*artifacts_proto.ThirdParty)
}

// Gets the tool information from the inventory. If the tool is not
// already downloaded, we download it and update the hashes.
func (self *InventoryService) GetToolInfo(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool string) (*artifacts_proto.Tool, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, item := range self.binaries.Tools {
		if item.Name == tool {
			if item.Hash == "" {
				// Try to download the item.
				err := self.downloadTool(ctx, config_obj, item)
				if err != nil {
					return nil, err
				}
			}
			return proto.Clone(item).(*artifacts_proto.Tool), nil
		}
	}
	return nil, errors.New(fmt.Sprintf("Tool %v not declared in inventory.", tool))
}

func (self *InventoryService) downloadTool(
	ctx context.Context,
	config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) error {
	if tool.Url == "" {
		return errors.New(fmt.Sprintf(
			"Tool %v has no url defined - upload it manually.", tool.Name))
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	path_manager := paths.NewInventoryPathManager(config_obj, tool)
	fd, err := file_store_factory.WriteFile(path_manager.Path())
	if err != nil {
		return err
	}
	defer fd.Close()

	fd.Truncate()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Downloading tool <green>%v</> FROM <red>%v</>", tool.Name,
		tool.Url)
	request, err := http.NewRequestWithContext(ctx, "GET", tool.Url, nil)
	res, err := self.Client.Do(request)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// If the download failed, we can not store this tool.
	if res.StatusCode != 200 {
		return errors.New("Unable to download file")
	}
	sha_sum := sha256.New()

	_, err = utils.Copy(ctx, fd, io.TeeReader(res.Body, sha_sum))
	if err == nil {
		tool.Hash = hex.EncodeToString(sha_sum.Sum(nil))
	}

	return self.db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) getGithubRelease(ctx context.Context,
	config_obj *config_proto.Config,
	tool *artifacts_proto.Tool) (string, error) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest",
		tool.GithubProject)
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	logger.Info("Resolving latest Github release for <green>%v</>", tool.Name)
	res, err := self.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", errors.New(fmt.Sprintf("Error: %v", res.Status))
	}

	response, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err,
			"While make Github API call to "+url)
	}

	api_obj := &githubReleasesAPI{}
	err = json.Unmarshal(response, &api_obj)
	if err != nil {
		return "", errors.Wrap(err,
			"While make Github API call to "+url)
	}

	release_re, err := regexp.Compile(tool.GithubAssetRegex)
	if err != nil {
		return "", err
	}

	for _, asset := range api_obj.Assets {
		if release_re.MatchString(asset.Name) {
			logger.Info("Tool <green>%v</> can be found at <cyan>%v</>",
				tool.Name, asset.BrowserDownloadUrl)
			return asset.BrowserDownloadUrl, nil
		}
	}

	return "", errors.New("Release not found from github API " + url)
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

func (self *InventoryService) AddTool(ctx context.Context,
	config_obj *config_proto.Config, tool_request *artifacts_proto.Tool) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// Make a copy to work on.
	tool := *tool_request

	// If we are downloading from github we have to resolve and
	// verify the binary URL now.
	if tool.GithubProject != "" {
		var err error
		tool.Url, err = self.getGithubRelease(ctx, config_obj, &tool)
		if err != nil {
			return errors.Wrap(
				err, "While resolving github release "+tool.GithubProject)
		}
	}

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// Obfuscate the public directory path.
	tool.FilestorePath = paths.ObfuscateName(config_obj, tool.Name)

	if tool.ServeLocally {
		if len(config_obj.Client.ServerUrls) == 0 {
			return errors.New("No server URLs configured!")
		}
		tool.ServeUrl = config_obj.Client.ServerUrls[0] + "public/" + tool.FilestorePath
	}

	if tool.Url == "" && tool.ServeUrl == "" {
		return errors.New("No tool URL defined and I will not be serving it locally!")
	}

	// Set the filename to something sensible so it is always valid.
	if tool.Filename == "" {
		if tool.Url != "" {
			tool.Filename = path.Base(tool.Url)
		} else {
			tool.Filename = path.Base(tool.ServeUrl)
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
	self.binaries = inventory

	return err
}

func NewDummy() *InventoryService {
	return &InventoryService{
		Clock: utils.RealClock{},
		db:    datastore.NewTestDataStore(),
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
}

func StartInventoryService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	inventory_service := NewDummy()

	if config_obj.Datastore == nil {
		services.RegisterInventory(inventory_service)
		return nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	inventory_service.db = db

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notification, cancel := services.ListenForNotification(
		constants.ThirdPartyInventory)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-notification:
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: ", err)
				}

			case <-time.After(time.Second):
				inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: ", err)
				}
			}

			cancel()
			notification, cancel = services.ListenForNotification(
				constants.ThirdPartyInventory)
		}
	}()

	logger.Info("<green>Starting</> Inventory Service")

	services.RegisterInventory(inventory_service)
	return inventory_service.LoadFromFile(config_obj)
}

func init() {
	services.RegisterInventory(NewDummy())
}
