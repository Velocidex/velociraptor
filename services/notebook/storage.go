package notebook

import (
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

type NotebookStore interface {
	SetNotebook(in *api_proto.NotebookMetadata) error
	GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error)
	SetNotebookCell(notebook_id string, in *api_proto.NotebookCell) error
	GetNotebookCell(notebook_id, cell_id string) (*api_proto.NotebookCell, error)
	StoreAttachment(notebook_id, filename string, data []byte) (api.FSPathSpec, error)

	UpdateShareIndex(notebook *api_proto.NotebookMetadata) error

	GetAvailableDownloadFiles(notebook_id string) (*api_proto.AvailableDownloads, error)
	GetAvailableTimelines(notebook_id string) []string
	GetAvailableUploadFiles(notebook_id string) (
		*api_proto.AvailableDownloads, error)
}

type NotebookStoreImpl struct {
	config_obj *config_proto.Config
}

func (self *NotebookStoreImpl) SetNotebook(in *api_proto.NotebookMetadata) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)
	return db.SetSubject(self.config_obj, notebook_path_manager.Path(), in)
}

func (self *NotebookStoreImpl) GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config_obj, notebook_path_manager.Path(),
		notebook)
	return notebook, err
}

func (self *NotebookStoreImpl) SetNotebookCell(
	notebook_id string, in *api_proto.NotebookCell) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_id).Cell(in.CellId)
	return db.SetSubject(self.config_obj, notebook_path_manager.Path(), in)
}

func (self *NotebookStoreImpl) GetNotebookCell(notebook_id, cell_id string) (*api_proto.NotebookCell, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id).
		Cell(cell_id)
	notebook_cell := &api_proto.NotebookCell{}
	err = db.GetSubject(self.config_obj, notebook_path_manager.Path(),
		notebook_cell)
	return notebook_cell, err
}

func (self *NotebookStoreImpl) StoreAttachment(notebook_id, filename string, data []byte) (api.FSPathSpec, error) {
	full_path := paths.NewNotebookPathManager(notebook_id).
		Attachment(filename)
	file_store_factory := file_store.GetFileStore(self.config_obj)
	fd, err := file_store_factory.WriteFile(full_path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	_, err = fd.Write(data)
	return full_path, err
}

// Update the notebook index for all the users and collaborators.
func (self *NotebookStoreImpl) UpdateShareIndex(
	notebook *api_proto.NotebookMetadata) error {

	// Flow notebooks and hunt notebooks are not indexable by the
	// general purpose notebook index because we can easily locate
	// them using the hunt id or the flow id.
	if nonIndexingRegex.MatchString(notebook.NotebookId) {
		return nil
	}

	if notebook.Creator == "" {
		return errors.New("A notebook creator must be specified")
	}

	users := append([]string{notebook.Creator}, notebook.Collaborators...)
	indexer, err := services.GetIndexer()
	if err != nil {
		return err
	}

	return indexer.SetSimpleIndex(self.config_obj, paths.NOTEBOOK_INDEX,
		notebook.NotebookId, users)
}
