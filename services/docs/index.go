package docs

import (
	"archive/zip"
	"context"

	"github.com/Velocidex/velociraptor-site-search/api"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	fs_api "www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *DocManager) getInventoryStat(ctx context.Context) (fs_api.FileInfo, error) {
	inventory_service, err := services.GetInventory(self.config_obj)
	if err != nil {
		return nil, err
	}

	// Materialize the tool because we need to check it's timestamp
	tool, err := inventory_service.GetToolInfo(ctx, self.config_obj,
		"DocsIndex", "")
	if err != nil {
		return nil, err
	}

	path_manager := paths.NewInventoryPathManager(self.config_obj, tool)
	path, file_store_factory, err := path_manager.Path()
	if err != nil {
		return nil, err
	}

	return file_store_factory.StatFile(path)
}

func (self *DocManager) shouldUnpackTool(ctx context.Context) (
	inventory_path, index_path fs_api.FSPathSpec, unpack bool, err error) {

	// Check the index on disk
	path_manager := paths.NewDocsIndexPathManager()
	file_store_factory := file_store.GetFileStore(self.config_obj)
	existing_stat, err := file_store_factory.StatFile(
		path_manager.Metadata())
	if err != nil {
		// Index does not exist, we need to unpack it.
		inv_stat, err := self.getInventoryStat(ctx)
		if err != nil {
			return nil, nil, true, err
		}
		return inv_stat.PathSpec(), path_manager.Index(), true, nil
	}

	// Index exists, we need to see if it is newer than the inventory
	// one.
	inv_stat, err := self.getInventoryStat(ctx)
	if err != nil {
		// We can not access the inventory for some reason, this is an
		// error.
		return nil, nil, true, err
	}

	// The existing index is later than the inventory file, we do not
	// need to refresh it.
	if existing_stat.ModTime().After(inv_stat.ModTime()) {
		logging.GetLogger(self.config_obj, &logging.GUIComponent).
			Info("<green>Docs Manager</>: Loading existing index")
		return inv_stat.PathSpec(), path_manager.Index(), false, nil
	}

	// We need to unpack the index.
	logging.GetLogger(self.config_obj, &logging.GUIComponent).
		Info("<green>Docs Manager</>: Existing index is too old, unpacking index from inventory")
	return inv_stat.PathSpec(), path_manager.Index(), true, nil
}

func (self *DocManager) unpackIndex(
	ctx context.Context,
	inventory_path, index_path fs_api.FSPathSpec) error {

	file_store_factory := file_store.GetFileStore(self.config_obj)
	reader, err := file_store_factory.ReadFile(inventory_path)
	if err != nil {
		return err
	}
	defer reader.Close()

	stat, err := reader.Stat()
	if err != nil {
		return err
	}

	zipfd, err := zip.NewReader(utils.MakeReaderAtter(reader), stat.Size())
	if err != nil {
		return err
	}

	for _, file := range zipfd.File {
		fd, err := file.Open()
		if err != nil {
			return err
		}

		w, err := file_store_factory.WriteFile(
			index_path.AddUnsafeChild(file.Name))
		if err != nil {
			return err
		}

		w.Truncate()

		_, err = utils.Copy(ctx, w, fd)
		if err != nil {
			return err
		}
		w.Close()
	}

	return nil
}

// Gets a working index.  If the index does not exist in the file
// store, we use the inventory service to fetch it.
func (self *DocManager) GetIndex(ctx context.Context) (res api.Index, err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.index != nil {
		return self.index, nil
	}

	inventory_path, index_path, unpack, err := self.shouldUnpackTool(ctx)
	if err != nil {
		return nil, err
	}

	if unpack {
		err = self.unpackIndex(ctx, inventory_path, index_path)
		if err != nil {
			return nil, err
		}
	}

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	// The raw underlying filename on disk.
	raw_filename := datastore.AsFilestoreFilename(
		db, self.config_obj, index_path)

	index, err := api.OpenIndex(raw_filename)
	if err != nil {
		return nil, err
	}

	self.index = index
	return index, err
}
