package notebook

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/reformat"
)

type NotebookManager struct {
	config_obj *config_proto.Config
	Store      NotebookStore
}

func (self *NotebookManager) GetNotebook(
	ctx context.Context, notebook_id string, include_uploads bool) (
	*api_proto.NotebookMetadata, error) {

	notebook, err := self.Store.GetNotebook(notebook_id)
	if err != nil {
		return nil, err
	}

	if include_uploads {
		// An error here just means there are no AvailableDownloads.
		notebook.AvailableDownloads, _ = self.Store.GetAvailableDownloadFiles(
			notebook_id)
		notebook.AvailableUploads, _ = self.Store.GetAvailableUploadFiles(
			notebook_id)
		notebook.Timelines = self.Store.GetAvailableTimelines(notebook_id)
	} else {
		notebook.AvailableUploads = nil
		notebook.AvailableDownloads = nil
		notebook.Timelines = nil
	}

	return notebook, nil
}

func (self *NotebookManager) NewNotebook(
	ctx context.Context, username string, in *api_proto.NotebookMetadata) (
	*api_proto.NotebookMetadata, error) {

	// Override these attributes
	in.Creator = username
	in.CreatedTime = utils.GetTime().Now().Unix()
	in.ModifiedTime = in.CreatedTime

	// Allow hunt notebooks to be created with a specified hunt ID.
	if !strings.HasPrefix(in.NotebookId, "N.H.") &&
		!strings.HasPrefix(in.NotebookId, "N.F.") &&
		!strings.HasPrefix(in.NotebookId, "N.E.") {
		in.NotebookId = NewNotebookId()
	}

	err := self.Store.SetNotebook(in)
	if err != nil {
		return nil, err
	}

	err = CreateInitialNotebook(ctx, self.config_obj, in, username)
	if err != nil {
		return nil, err
	}

	// Add the new notebook to the index so it can be seen. Only
	// non-hunt notebooks are searchable in the index since the
	// hunt notebooks are always found in the hunt results.
	err = self.Store.UpdateShareIndex(in)
	if err != nil {
		return nil, err
	}

	return in, err
}

func (self *NotebookManager) UpdateNotebook(
	ctx context.Context, in *api_proto.NotebookMetadata) error {

	err := self.Store.SetNotebook(in)
	if err != nil {
		return err
	}

	return self.Store.UpdateShareIndex(in)
}

func (self *NotebookManager) GetNotebookCell(ctx context.Context,
	notebook_id, cell_id string) (*api_proto.NotebookCell, error) {

	notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id)

	// Cell does not exist, make it a default cell.
	if errors.Is(err, os.ErrNotExist) {
		return &api_proto.NotebookCell{
			Input:  "",
			Output: "",
			Data:   "{}",
			CellId: cell_id,
			Type:   "Markdown",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return notebook_cell, nil
}

// Cancel a current operation
func (self *NotebookManager) CancelNotebookCell(
	ctx context.Context, notebook_id, cell_id string) error {

	// Unset the calculating bit in the notebook in case the
	// renderer is not actually running (e.g. server restart).
	notebook_cell, err := self.Store.GetNotebookCell(notebook_id, cell_id)
	if err != nil || notebook_cell.CellId != cell_id {
		return errors.New("No such cell")
	}

	// Switch the cell into not calculating - this will force all
	// workers to exit.
	notebook_cell.Calculating = false
	notebook_cell.Error = "Cancelled!"
	// Make sure we write the cancel message ASAP
	err = self.Store.SetNotebookCell(notebook_id, notebook_cell)
	if err != nil {
		return err
	}

	// Notify the calculator immediately if we are in the same
	// process. This makes it more responsive.
	notifier, err := services.GetNotifier(self.config_obj)
	if err != nil {
		return err
	}
	return notifier.NotifyListener(
		ctx, self.config_obj, cell_id, "CancelNotebookCell")
}

func (self *NotebookManager) UploadNotebookAttachment(ctx context.Context,
	in *api_proto.NotebookFileUploadRequest) (
	*api_proto.NotebookFileUploadResponse, error) {
	decoded, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return nil, err
	}

	filename := NewNotebookAttachmentId() + in.Filename

	full_path, err := self.Store.StoreAttachment(
		in.NotebookId, filename, decoded)
	if err != nil {
		return nil, err
	}

	result := &api_proto.NotebookFileUploadResponse{
		Url: full_path.AsClientPath() + "?org_id=" +
			url.QueryEscape(utils.NormalizedOrgId(self.config_obj.OrgId)),
	}
	return result, nil
}

func NewNotebookManager(
	config_obj *config_proto.Config,
	storage NotebookStore) *NotebookManager {
	result := &NotebookManager{
		config_obj: config_obj,
		Store:      storage,
	}
	return result
}

func NewNotebookManagerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.NotebookManager, error) {

	notebook_service := NewNotebookManager(config_obj,
		&NotebookStoreImpl{
			config_obj: config_obj,
		})

	return notebook_service, notebook_service.Start(ctx, config_obj, wg)
}

func (self *NotebookManager) ReformatVQL(
	ctx context.Context, vql string) (string, error) {

	scope := vql_subsystem.MakeScope()
	return reformat.ReFormatVQL(scope, vql, vfilter.DefaultFormatOptions)
}
