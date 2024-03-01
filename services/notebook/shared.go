package notebook

import (
	"context"
	"regexp"
	"sort"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
	ctx context.Context, user string, offset, count uint64) (
	[]*api_proto.NotebookMetadata, error) {

	result := []*api_proto.NotebookMetadata{}

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

func (self *NotebookManager) GetAllNotebooks() (
	[]*api_proto.NotebookMetadata, error) {
	return self.Store.GetAllNotebooks()
}
