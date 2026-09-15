package notebook

import (
	"context"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *NotebookManager) CheckNotebookAccess(
	notebook *api_proto.NotebookMetadata, user string) bool {
	return checkNotebookAccess(notebook, user)
}

func checkNotebookAccess(notebook *api_proto.NotebookMetadata, user string) bool {
	if notebook.Public {
		return true
	}

	return notebook.Creator == user || utils.InString(notebook.Collaborators, user)
}

func (self *NotebookManager) GetSharedNotebooks(
	ctx context.Context, username string) (api.FSPathSpec, error) {
	return self.Store.GetSharedNotebooks(ctx, username)
}

// Returns a result set of all the notebooks which are either owned or
// shared with the user. This is used in the GUI to show the user's
// notebook view using the standard table widget. This code keeps it
// in sync with the global notebooks view.
func (self *NotebookStoreImpl) GetSharedNotebooks(
	ctx context.Context, username string) (api.FSPathSpec, error) {

	notebook_path_manager := paths.NewNotebookPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	index_filename := notebook_path_manager.NotebookIndexForUser(username)

	// Check if the file exists and is fresh enough
	_, err := file_store_factory.StatFile(index_filename)
	if err == nil {
		self.mu.Lock()
		last_user_version, pres := self.last_shared_results[username]
		self.mu.Unlock()
		if pres && last_user_version >= self.last_version {
			return index_filename, nil
		}
	}

	logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
	logger.Debug("Building notebook index for %v\n", username)

	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, index_filename,
		json.DefaultEncOpts(), utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return nil, err
	}
	defer rs_writer.Close()

	all_notebooks, err := self.GetAllNotebooks(ctx,
		services.NotebookSearchOptions{
			Username: username,
		})
	if err != nil {
		return nil, err
	}

	sort.Slice(all_notebooks, func(i, j int) bool {
		return all_notebooks[i].NotebookId > all_notebooks[j].NotebookId
	})

	for _, notebook := range all_notebooks {
		rs_writer.Write(ordereddict.NewDict().
			Set("NotebookId", notebook.NotebookId).
			Set("Name", notebook.Name).
			Set("Description", notebook.Description).
			Set("Creation Time", time.Unix(notebook.CreatedTime, 0)).
			Set("Modified Time", time.Unix(notebook.ModifiedTime, 0)).
			Set("Creator", notebook.Creator).
			Set("Collaborators", notebook.Collaborators))
	}

	self.mu.Lock()
	self.last_shared_results[username] = self.last_version
	self.mu.Unlock()

	return index_filename, nil
}

func (self *NotebookManager) GetAllNotebooks(ctx context.Context,
	opts services.NotebookSearchOptions) ([]*api_proto.NotebookMetadata, error) {
	return self.Store.GetAllNotebooks(ctx, opts)
}
