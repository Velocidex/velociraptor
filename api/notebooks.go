package api

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	errors "github.com/go-errors/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to read notebooks.")
	}

	result := &api_proto.Notebooks{}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// We want a single notebook metadata.
	if in.NotebookId != "" {
		notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId)
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
			logging.LogAudit(org_config_obj, principal, "notebook not shared.",
				logrus.Fields{
					"action":   "Access Denied",
					"notebook": in.NotebookId,
					"error":    err.Error(),
				})
			return nil, InvalidStatus("User has no access to this notebook")
		}

		result.Items = append(result.Items, notebook_metadata)
		return result, nil
	}

	notebooks, err := notebook_manager.GetSharedNotebooks(ctx,
		principal, in.Offset, in.Count)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result.Items = notebooks
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
		return nil, status.Error(codes.PermissionDenied,
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
		return nil, status.Error(codes.PermissionDenied,
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	// If the notebook is not properly shared with the user they
	// may not edit it.
	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	old_notebook, err := notebook_manager.GetNotebook(ctx, in.NotebookId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(old_notebook, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	if old_notebook.ModifiedTime != in.ModifiedTime {
		return nil, InvalidStatus("Edit clash detected.")
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

func (self *ApiServer) GetNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("GetNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NoteboookCellId")
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to read notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	return notebook_manager.GetNotebookCell(ctx, in.NotebookId, in.CellId)
}

func (self *ApiServer) UpdateNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("UpdateNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NoteboookCellId")
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Check that the user has access to this notebook.
	notebook_metadata, err := notebook_manager.GetNotebook(ctx, in.NotebookId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if !notebook_manager.CheckNotebookAccess(notebook_metadata, principal) {
		return nil, InvalidStatus("Notebook is not shared with user.")
	}

	return notebook_manager.UpdateNotebookCell(
		ctx, notebook_metadata, principal, in)
}

func (self *ApiServer) CancelNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*emptypb.Empty, error) {

	defer Instrument("CancelNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, InvalidStatus("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, InvalidStatus("Invalid NoteboookCellId")
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &emptypb.Empty{}, notebook_manager.CancelNotebookCell(
		ctx, in.NotebookId, in.CellId)
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	notebook_manager, err := services.GetNotebookManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	return notebook_manager.UploadNotebookAttachment(ctx, in)
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
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to export notebooks.")
	}

	switch in.Type {
	case "zip":
		return &emptypb.Empty{}, exportZipNotebook(
			org_config_obj, in.NotebookId, principal)
	default:
		return &emptypb.Empty{}, exportHTMLNotebook(
			org_config_obj, in.NotebookId, principal)
	}
}

// Create a portable notebook into a zip file.
func exportZipNotebook(
	config_obj *config_proto.Config,
	notebook_id, principal string) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	notebook := &api_proto.NotebookMetadata{}
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return err
	}
	if !notebook_manager.CheckNotebookAccess(notebook, principal) {
		return InvalidStatus("Notebook is not shared with user.")
	}

	filename := notebook_path_manager.ZipExport()

	// Allow 1 hour to export the notebook.
	sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	go func() {
		defer cancel()

		err := reporting.ExportNotebookToZip(
			sub_ctx, config_obj, notebook_path_manager)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"export_file": filename,
				"error":       err.Error(),
			}).Error("CreateNotebookDownloadFile")
			return
		}
	}()

	return nil
}

func exportHTMLNotebook(config_obj *config_proto.Config,
	notebook_id, principal string) error {
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	notebook := &api_proto.NotebookMetadata{}
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		return err
	}

	if !notebook_manager.CheckNotebookAccess(notebook, principal) {
		return InvalidStatus("Notebook is not shared with user.")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	filename := notebook_path_manager.HtmlExport()

	writer, err := file_store_factory.WriteFile(filename)
	if err != nil {
		return err
	}

	sha_sum := sha256.New()
	md5_sum := md5.New()
	tee_writer := utils.NewTee(writer, sha_sum, md5_sum)

	stats := &api_proto.ContainerStats{
		Timestamp:  uint64(time.Now().Unix()),
		Type:       "html",
		Components: path_specs.AsGenericComponentList(filename),
	}
	stats_path := notebook_path_manager.PathStats(filename)

	err = db.SetSubject(config_obj, stats_path, stats)
	if err != nil {
		return err
	}

	// Allow 1 hour to export the notebook.
	sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	go func() {
		defer writer.Close()
		defer cancel()

		defer func() {
			stats.Hash = hex.EncodeToString(sha_sum.Sum(nil))
			stats.TotalDuration = uint64(time.Now().Unix()) - stats.Timestamp

			db.SetSubject(config_obj, stats_path, stats)
		}()

		err := reporting.ExportNotebookToHTML(
			sub_ctx, config_obj, notebook.NotebookId, tee_writer)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"export_file": filename,
				"error":       err.Error(),
			}).Error("CreateNotebookDownloadFile")
			return
		}
	}()

	return nil
}
