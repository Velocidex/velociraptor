package notebook

import (
	"context"
	"regexp"
	"sort"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	// Notebook ids that are not indexed. These are flow, hunt and
	// event notebooks.
	nonIndexingRegex = regexp.MustCompile(`^N\.[EFH]\.`)
)

func (self *NotebookManager) CheckNotebookAccess(
	notebook *api_proto.NotebookMetadata,
	user string) bool {
	if notebook.Public {
		return true
	}

	return notebook.Creator == user || utils.InString(notebook.Collaborators, user)
}

// Returns all the notebooks which are either owned or shared with the
// user. This view is only called from the global notebook view so it
// only needs to return a brief version of the notebooks - it does not
// include uploads and timelines.
func (self *NotebookManager) GetSharedNotebooks(
	ctx context.Context, user string,
	options result_sets.ResultSetOptions,
	offset, count uint64) (
	[]*api_proto.NotebookMetadata, error) {

	result := []*api_proto.NotebookMetadata{}

	err := self.buildNotebookIndex(ctx, user)
	if err != nil {
		return nil, err
	}

	all_notebooks, err := self.GetAllNotebooks()
	if err != nil {
		return nil, err
	}
	for _, notebook := range all_notebooks {
		if !self.CheckNotebookAccess(notebook, user) {
			continue
		}

		if !notebook.Hidden && notebook.NotebookId != "" {
			result = append(result, notebook)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].NotebookId < result[j].NotebookId
	})

	return result, nil
}

func (self *NotebookManager) buildNotebookIndex(
	ctx context.Context, username string) error {

	notebook_path_manager := paths.NewNotebookPathManager("")
	file_store_factory := file_store.GetFileStore(self.config_obj)
	index_filename := notebook_path_manager.NotebookIndexForUser(username)

	stat, err := file_store_factory.StatFile(index_filename)

	if err == nil && stat.ModTime().Unix() >= self.Store.Version() {
		return nil
	}

	logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
	logger.Debug("Building notebook index for %v\n", username)

	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, index_filename,
		json.DefaultEncOpts(), utils.SyncCompleter,
		result_sets.TruncateMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	all_notebooks, err := self.GetAllNotebooks()
	if err != nil {
		return err
	}

	sort.Slice(all_notebooks, func(i, j int) bool {
		return all_notebooks[i].NotebookId < all_notebooks[j].NotebookId
	})

	for _, notebook := range all_notebooks {
		if notebook.Hidden ||
			!self.CheckNotebookAccess(notebook, username) {
			continue
		}

		rs_writer.Write(ordereddict.NewDict().
			Set("NotebookId", notebook.NotebookId).
			Set("Name", notebook.Name).
			Set("Description", notebook.Description).
			Set("Creation Time", time.Unix(0, notebook.CreatedTime)).
			Set("Modified Time", time.Unix(0, notebook.ModifiedTime)).
			Set("Creator", notebook.Creator).
			Set("Collaborators", notebook.Collaborators))
	}

	return nil
}

func (self *NotebookManager) GetAllNotebooks() (
	[]*api_proto.NotebookMetadata, error) {
	return self.Store.GetAllNotebooks()
}
