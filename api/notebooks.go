package api

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql/server/notebooks"
)

const (
	SKIP_UPLOADS = false
)

// Get all the current user's notebooks and those notebooks shared
// with them.
func (self *ApiServer) GetNotebooks(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.Notebooks, error) {

	defer Instrument("GetNotebooks")()

	// Empty creators are called internally.
	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err, "User is not allowed to read notebooks.")
	}

	result := &api_proto.Notebooks{}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// List all the timelines
	if in.IncludeTimelines {
		// This is only called for global notebooks because client and
		// hunt notebooks always specify the exact notebook id.
		notebooks, err := notebook_manager.GetAllNotebooks(ctx,
			services.NotebookSearchOptions{
				Username:  principal,
				Timelines: true,
			})
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		for _, n := range notebooks {
			result.Items = append(result.Items,
				proto.Clone(n).(*api_proto.NotebookMetadata))

			if uint64(len(result.Items)) > in.Count {
				break
			}
		}
		return result, nil
	}

	if in.NotebookId == "" {
		return nil, Status(self.verbose, errors.New("NotebookId must be specified"))
	}

	notebook_metadata, err := notebook_manager.GetNotebook(
		ctx, in.NotebookId, in.IncludeUploads)
	// Handle the EOF especially: it means there is no such
	// notebook and return an empty result set.
	if errors.Is(err, os.ErrNotExist) ||
		(notebook_metadata != nil && notebook_metadata.NotebookId == "") {
		return result, nil
	}

	if err != nil {
		logging.GetLogger(
			org_config_obj, &logging.FrontendComponent).
			Error("Unable to open notebook: %v", err)
		return nil, Status(self.verbose, err)
	}

	// Document not owned or collaborated with.
	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		err := services.LogAudit(ctx,
			org_config_obj, principal, "notebook not shared.",
			ordereddict.NewDict().
				Set("action", "Access Denied").
				Set("notebook", in.NotebookId))
		if err != nil {
			logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
			logger.Error("<red>notebook not shared</> %v %v", principal, in.NotebookId)
		}

		return nil, InvalidStatus("User has no access to this notebook")
	}

	result.Items = append(result.Items, notebook_metadata)
	return result, nil
}

func (self *ApiServer) NewNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	defer Instrument("NewNotebook")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to create notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return notebook_manager.NewNotebook(ctx, principal, in)
}

func (self *ApiServer) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (
	*api_proto.NotebookMetadata, error) {

	defer Instrument("NewNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	return notebook_manager.NewNotebookCell(ctx, in, principal)
}

func (self *ApiServer) UpdateNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	defer Instrument("UpdateNotebook")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	// If the notebook is not properly shared with the user they
	// may not edit it.
	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	old_notebook, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(old_notebook, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	// When updating an existing notebook only certain fields may
	// be changed by the user - definitely not the creator, created time or notebookId.
	in.ModifiedTime = time.Now().Unix()
	in.Creator = old_notebook.Creator
	in.CreatedTime = old_notebook.CreatedTime
	in.NotebookId = old_notebook.NotebookId

	// Filter out any empty cells.
	cell_metadata := make([]*api_proto.NotebookCell, 0, len(in.CellMetadata))
	for i := 0; i < len(in.CellMetadata); i++ {
		cell := in.CellMetadata[i]
		if cell.CellId != "" {
			cell_metadata = append(cell_metadata, cell)
		}
	}
	in.CellMetadata = cell_metadata

	return in, notebook_manager.UpdateNotebook(ctx, in)
}

func (self *ApiServer) DeleteNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*emptypb.Empty, error) {

	defer Instrument("DeleteNotebook")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to delete notebooks.")
	}

	// If the notebook is not properly shared with the user they
	// may not edit it.
	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	old_notebook, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(old_notebook, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	err = notebook_manager.DeleteNotebook(ctx, in.NotebookId, nil,
		true /* really_do_it */)

	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) GetNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("GetNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NotebookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NotebookCellId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to read notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	return notebook_manager.GetNotebookCell(ctx, in.NotebookId, in.CellId, in.Version)
}

func (self *ApiServer) UpdateNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("UpdateNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NotebookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NotebookCellId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Check that the user has access to this notebook.
	notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	res, err := notebook_manager.UpdateNotebookCell(
		ctx, notebook_metadata, principal, in)
	return res, Status(self.verbose, err)
}

func (self *ApiServer) RevertNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("RevertNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NotebookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NotebookCellId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Check that the user has access to this notebook.
	notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	res, err := notebook_manager.RevertNotebookCellVersion(
		ctx, in.NotebookId, in.CellId, in.Version)
	return res, Status(self.verbose, err)
}

func (self *ApiServer) CancelNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*emptypb.Empty, error) {

	defer Instrument("CancelNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NotebookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NotebookCellId")
	}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &emptypb.Empty{}, notebook_manager.CancelNotebookCell(
		ctx, in.NotebookId, in.CellId, in.Version)
}

func (self *ApiServer) UploadNotebookAttachment(
	ctx context.Context,
	in *api_proto.NotebookFileUploadRequest) (*api_proto.NotebookFileUploadResponse, error) {

	defer Instrument("UploadNotebookAttachment")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	res, err := notebook_manager.UploadNotebookAttachment(ctx, in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	return res, nil
}

func (self *ApiServer) CreateNotebookDownloadFile(
	ctx context.Context,
	in *api_proto.NotebookExportRequest) (*emptypb.Empty, error) {

	defer Instrument("CreateNotebookDownloadFile")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.PREPARE_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to export notebooks.")
	}

	wg := &sync.WaitGroup{}

	switch in.Type {
	case "zip":
		_, err := notebooks.ExportNotebookToZip(ctx,
			org_config_obj, wg, in.NotebookId,
			principal, in.PreferredName)

		return &emptypb.Empty{}, Status(self.verbose, err)

	default:
		_, err := notebooks.ExportNotebookToHTML(
			org_config_obj, wg, in.NotebookId,
			principal, in.PreferredName)
		return &emptypb.Empty{}, Status(self.verbose, err)
	}
}

func (self *ApiServer) RemoveNotebookAttachment(
	ctx context.Context,
	in *api_proto.NotebookFileUploadRequest) (*emptypb.Empty, error) {

	defer Instrument("RemoveNotebookAttachment")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.PREPARE_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to update notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	notebook, err := notebook_manager.GetNotebook(ctx, in.NotebookId, SKIP_UPLOADS)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	return &emptypb.Empty{}, notebook_manager.RemoveNotebookAttachment(ctx,
		in.NotebookId, in.Components)
}
