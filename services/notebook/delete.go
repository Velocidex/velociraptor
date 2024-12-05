package notebook

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func (self *NotebookManager) DeleteNotebook(ctx context.Context,
	notebook_id string, progress chan vfilter.Row,
	really_do_it bool) error {
	return self.Store.DeleteNotebook(ctx, notebook_id, progress, really_do_it)
}

func (self *NotebookStoreImpl) DeleteNotebook(ctx context.Context,
	notebook_id string, progress chan vfilter.Row,
	really_do_it bool) error {

	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)

	if really_do_it {
		err = db.DeleteSubjectWithCompletion(
			self.config_obj, notebook_path_manager.Path(),
			utils.SyncCompleter)
		if err != nil {
			return err
		}

		// Also remove it from our local cache.
		self.mu.Lock()
		delete(self.global_notebooks, notebook_id)
		self.last_deleted = utils.GetTime().Now().Unix()
		self.mu.Unlock()
	}

	// Indiscriminately delete all the notebook's datastore files.
	err = datastore.Walk(
		self.config_obj, db, notebook_path_manager.DSDirectory(),
		datastore.WalkWithoutDirectories,
		func(filename api.DSPathSpec) error {
			if progress != nil {
				select {
				case <-ctx.Done():
					return nil

				case progress <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Notebook").
					Set("vfs_path", filename):
				}
			}

			if really_do_it {
				err = db.DeleteSubject(self.config_obj, filename)
				if err != nil {
					return err
				}
			}

			return nil
		})
	if err != nil {
		return err
	}

	// Delete the filestore files.
	err = api.Walk(file_store_factory,
		notebook_path_manager.Directory(),
		func(filename api.FSPathSpec, info os.FileInfo) error {
			if progress != nil {
				select {
				case <-ctx.Done():
					return nil

				case progress <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Filestore").
					Set("vfs_path", filename):
				}
			}

			if really_do_it {
				err = file_store_factory.Delete(filename)
				if err != nil && progress != nil {
					select {
					case <-ctx.Done():
						return nil

					case progress <- ordereddict.NewDict().
						Set("notebook_id", notebook_id).
						Set("type", "Filestore").
						Set("vfs_path", filename).
						Set("Error", err.Error()):
					}
				}
			}
			return nil
		})

	return err
}
