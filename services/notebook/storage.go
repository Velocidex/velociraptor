package notebook

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	IGNORE_REPORT chan *ordereddict.Dict = nil
)

type NotebookStore interface {
	SetNotebook(in *api_proto.NotebookMetadata) error
	GetNotebook(notebook_id string) (*api_proto.NotebookMetadata, error)
	SetNotebookCell(notebook_id string, in *api_proto.NotebookCell) error
	GetNotebookCell(notebook_id, cell_id, version string) (*api_proto.NotebookCell, error)

	// progress_chan receives information about deletion. It may be
	// nil if callers dont care about it.
	RemoveNotebookCell(
		ctx context.Context, config_obj *config_proto.Config,
		notebook_id, cell_id, version string, progress_chan chan *ordereddict.Dict) error

	StoreAttachment(notebook_id, filename string, data []byte) (api.FSPathSpec, error)
	RemoveAttachment(ctx context.Context, notebook_id string, components []string) error

	UpdateShareIndex(notebook *api_proto.NotebookMetadata) error

	GetAvailableDownloadFiles(notebook_id string) (*api_proto.AvailableDownloads, error)
	GetAvailableTimelines(notebook_id string) []string
	GetAvailableUploadFiles(notebook_id string) (
		*api_proto.AvailableDownloads, error)
}

type NotebookStoreImpl struct {
	config_obj *config_proto.Config
}

func NewNotebookStore(config_obj *config_proto.Config) *NotebookStoreImpl {
	return &NotebookStoreImpl{config_obj: config_obj}
}

func (self *NotebookStoreImpl) SetNotebook(in *api_proto.NotebookMetadata) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)

	// Ensure the notebook reflects the last time it was set.
	in.ModifiedTime = utils.GetTime().Now().Unix()
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
		notebook_id).Cell(in.CellId, in.CurrentVersion)
	err = db.SetSubject(self.config_obj, notebook_path_manager.Path(), in)
	if err != nil {
		return err
	}

	// Open the notebook and update the cell's timestamp.
	notebook, err := self.GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	now := utils.GetTime().Now().Unix()

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	found := false
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			// Replace the cell with the new cell
			cell_md = proto.Clone(in).(*api_proto.NotebookCell)
			cell_md.Timestamp = now
			found = true
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	if !found {
		cell_md := proto.Clone(in).(*api_proto.NotebookCell)
		cell_md.Timestamp = now
		new_cell_md = append(new_cell_md, in)
	}

	notebook.CellMetadata = new_cell_md
	return self.SetNotebook(notebook)
}

func (self *NotebookStoreImpl) RemoveNotebookCell(
	ctx context.Context, config_obj *config_proto.Config,
	notebook_id, cell_id, version string, output_chan chan *ordereddict.Dict) error {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_id).Cell(cell_id, version)

	// Indiscriminately delete all the client's datastore files.
	err = datastore.Walk(config_obj, db, notebook_path_manager.DSDirectory(),
		datastore.WalkWithoutDirectories,
		func(filename api.DSPathSpec) error {
			if output_chan != nil {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Notebook").
					Set("vfs_path", filename):
				}
			}

			return db.DeleteSubject(config_obj, filename)
		})
	if err != nil {
		return err
	}

	// Remove the empty directories
	err = datastore.Walk(config_obj, db, notebook_path_manager.DSDirectory(),
		datastore.WalkWithDirectories,
		func(filename api.DSPathSpec) error {
			db.DeleteSubject(config_obj, filename)
			return nil
		})

	// Delete the filestore files.
	file_store_factory := file_store.GetFileStore(config_obj)
	err = api.Walk(file_store_factory, notebook_path_manager.Directory(),
		func(filename api.FSPathSpec, info os.FileInfo) error {
			if output_chan != nil {
				select {
				case <-ctx.Done():
					return nil

				case output_chan <- ordereddict.NewDict().
					Set("notebook_id", notebook_id).
					Set("type", "Filestore").
					Set("vfs_path", filename):
				}
			}
			return file_store_factory.Delete(filename)
		})
	if err != nil {
		return err
	}

	// Open the notebook and remove the cell
	notebook, err := self.GetNotebook(notebook_id)
	if err != nil {
		return err
	}

	now := utils.GetTime().Now().Unix()

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == cell_id {
			new_versions := make([]string, 0, len(cell_md.AvailableVersions))
			for _, cell_version := range cell_md.AvailableVersions {
				if cell_version != version {
					new_versions = append(new_versions, cell_version)
				}
			}
			cell_md.AvailableVersions = new_versions
			cell_md.Timestamp = now
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	notebook.CellMetadata = new_cell_md
	return self.SetNotebook(notebook)
}

func (self *NotebookStoreImpl) GetNotebookCell(
	notebook_id, cell_id, version string) (*api_proto.NotebookCell, error) {
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id).
		Cell(cell_id, version)
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
	indexer, err := services.GetIndexer(self.config_obj)
	if err != nil {
		return err
	}

	return indexer.SetSimpleIndex(self.config_obj, paths.NOTEBOOK_INDEX,
		notebook.NotebookId, users)
}
