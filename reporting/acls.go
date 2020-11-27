package reporting

import (
	"regexp"
	"sort"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	nonIndexingRegex = regexp.MustCompile(`^N\.[FH]\.`)
)

func CheckNotebookAccess(
	notebook *api_proto.NotebookMetadata,
	user string) bool {
	if notebook.Public {
		return true
	}

	return notebook.Creator == user || utils.InString(notebook.Collaborators, user)
}

// Returns all the notebooks which are either owned or shared with the
// user
func GetSharedNotebooks(
	config_obj *config_proto.Config,
	user string,
	offset, count uint64) ([]*api_proto.NotebookMetadata, error) {

	result := []*api_proto.NotebookMetadata{}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// Return all the notebooks from the index that potentially
	// could be shared with the user.
	for idx, notebook_id := range db.SearchClients(
		config_obj, constants.NOTEBOOK_INDEX, user, "",
		offset, count, datastore.SORT_UP) {
		if uint64(idx) < offset {
			continue
		}

		if uint64(idx) > offset+count {
			break
		}

		notebook_path_manager := NewNotebookPathManager(notebook_id)
		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
		if err != nil {
			logging.GetLogger(
				config_obj, &logging.FrontendComponent).
				Error("Unable to open notebook: %v", err)
			continue
		}

		if !CheckNotebookAccess(notebook, user) {
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

func GetAllNotebooks(
	config_obj *config_proto.Config) ([]*api_proto.NotebookMetadata, error) {
	result := []*api_proto.NotebookMetadata{}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	// List all available notebooks
	notebook_urns, err := db.ListChildren(
		config_obj, NotebookDir(), 0, 1000000)
	if err != nil {
		return nil, err
	}

	for _, urn := range notebook_urns {
		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(config_obj, urn, notebook)
		if err != nil {
			continue
		}
		result = append(result, notebook)
	}

	return result, nil
}

// Update the notebook index for all the users and collaborators.
func UpdateShareIndex(
	config_obj *config_proto.Config,
	notebook *api_proto.NotebookMetadata) error {

	// Flow notebooks and hunt notebooks are not indexable by the
	// general purpose notebook index because we can easily locate
	// them using the hunt id or the flow id.
	if nonIndexingRegex.MatchString(notebook.NotebookId) {
		return nil
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	users := append([]string{notebook.Creator}, notebook.Collaborators...)
	return db.SetIndex(config_obj, constants.NOTEBOOK_INDEX,
		notebook.NotebookId, users)
}
