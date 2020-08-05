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

type InventoryService struct {
	mu       sync.Mutex
	binaries *artifacts_proto.ThirdParty
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

	res, err := http.Get(tool.Url)
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

	// Save the hash for next time.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
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

	// Save the hash for next time.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) AddTool(
	config_obj *config_proto.Config, tool *artifacts_proto.Tool) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.binaries == nil {
		self.binaries = &artifacts_proto.ThirdParty{}
	}

	// Obfuscate the public directory path.
	tool.FilestorePath = paths.ObfuscateName(config_obj, tool.Name)

	if tool.ServeLocally {
		if len(config_obj.Client.ServerUrls) == 0 {
			return errors.New("No server URLs configured!")
		}
		tool.Url = config_obj.Client.ServerUrls[0] + "public/" + tool.FilestorePath
	}

	if tool.Url == "" {
		return errors.New("No tool URL defined and I will not be serving it locally!")
	}

	// Set the filename to something sensible so it is always valid.
	if tool.Filename == "" {
		tool.Filename = path.Base(tool.Url)
	}

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

	self.binaries.Version = uint64(time.Now().UnixNano())

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}
	return db.SetSubject(config_obj, constants.ThirdPartyInventory, self.binaries)
}

func (self *InventoryService) LoadFromFile(config_obj *config_proto.Config) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	inventory := &artifacts_proto.ThirdParty{}
	err = db.GetSubject(config_obj, constants.ThirdPartyInventory, inventory)
	self.binaries = inventory

	return nil
}

func StartInventoryService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) error {

	if config_obj.Datastore == nil {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	notification, cancel := services.ListenForNotification(
		constants.ThirdPartyInventory)
	defer cancel()

	inventory_service := &InventoryService{}

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
				err := inventory_service.LoadFromFile(config_obj)
				if err != nil {
					logger.Error("StartInventoryService: ", err)
				}
			}

			cancel()
			notification, cancel = services.ListenForNotification(
				constants.ThirdPartyInventory)
		}
	}()

	logger.Info("Starting Thirdparty Inventory Service")

	services.RegisterInventory(inventory_service)
	return inventory_service.LoadFromFile(config_obj)
}
