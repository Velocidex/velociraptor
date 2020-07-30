package api

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) ExportNotebook(
	ctx context.Context,
	in *api_proto.NotebookExportRequest) (*empty.Empty, error) {
	return nil, errors.New("not implementated")
}

// Get all the current user's notebooks and those notebooks shared
// with them.
func (self *ApiServer) GetNotebooks(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.Notebooks, error) {

	// Empty creators are called internally.
	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to read notebooks.")
	}

	result := &api_proto.Notebooks{}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// We want a single notebook metadata.
	if in.NotebookId != "" {
		notebook_path_manager := reporting.NewNotebookPathManager(
			in.NotebookId)
		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(self.config, notebook_path_manager.Path(),
			notebook)
		if err != nil {
			logging.GetLogger(
				self.config, &logging.FrontendComponent).
				Error("Unable to open notebook", err)
			return nil, err
		}

		// An error here just means there are no AvailableDownloads.
		notebook.AvailableDownloads, _ = getAvailableDownloadFiles(self.config,
			path.Join("/downloads/notebooks/", in.NotebookId))
		result.Items = append(result.Items, notebook)

		// Document not owned or collaborated with.
		if !reporting.CheckNotebookAccess(notebook, user_record.Name) {
			logging.GetLogger(
				self.config, &logging.Audit).WithFields(
				logrus.Fields{
					"user":     user_record.Name,
					"action":   "Access Denied",
					"notebook": in.NotebookId,
				}).
				Error("notebook not shared.", err)
			return nil, errors.New("User has no access to this notebook")
		}

		return result, nil
	}

	notebooks, err := reporting.GetSharedNotebooks(self.config, user_record.Name,
		in.Offset, in.Count)
	if err != nil {
		return nil, err
	}

	result.Items = notebooks
	return result, nil
}

func NewNotebookId() string {
	buf := make([]byte, 8)
	rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "N." + result
}

func NewNotebookAttachmentId() string {
	buf := make([]byte, 8)
	rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NA." + result
}

func NewNotebookCellId() string {
	buf := make([]byte, 8)
	rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NC." + result
}

func (self *ApiServer) NewNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*empty.Empty, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to create notebooks.")
	}

	in.Creator = user_name
	in.CreatedTime = time.Now().Unix()
	in.ModifiedTime = in.CreatedTime
	in.NotebookId = NewNotebookId()

	new_cell_id := NewNotebookCellId()

	in.CellMetadata = append(in.CellMetadata, &api_proto.NotebookCell{
		CellId:    new_cell_id,
		Timestamp: time.Now().Unix(),
	})

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	notebook_path_manager := reporting.NewNotebookPathManager(in.NotebookId)
	err = db.SetSubject(self.config, notebook_path_manager.Path(), in)

	// Add a new single cell to the notebook.
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:            fmt.Sprintf("# %s\n\n%s\n", in.Name, in.Description),
		NotebookId:       in.NotebookId,
		CellId:           new_cell_id,
		Type:             "Markdown",
		CurrentlyEditing: true,
	}

	// Add the new notebook to the index so it can be seen.
	err = reporting.UpdateShareIndex(self.config, in)
	if err != nil {
		return nil, err
	}

	_, err = self.UpdateNotebookCell(ctx, new_cell_request)
	return &empty.Empty{}, err
}

func (self *ApiServer) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookMetadata, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// Set a default artifact.
	if in.Input == "" && in.Type == "Artifact" {
		in.Input = default_artifact
	}

	notebook := &api_proto.NotebookMetadata{}
	notebook_path_manager := reporting.NewNotebookPathManager(in.NotebookId)
	err = db.GetSubject(self.config, notebook_path_manager.Path(), notebook)
	if err != nil {
		return nil, err
	}

	new_cell_md := []*api_proto.NotebookCell{}
	added := false

	notebook.LatestCellId = NewNotebookCellId()

	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    cell_md.CellId,
				Timestamp: time.Now().Unix(),
			})
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    notebook.LatestCellId,
				Timestamp: time.Now().Unix(),
			})
			added = true
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	// Add it to the end of the document.
	if !added {
		new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
			CellId:    notebook.LatestCellId,
			Timestamp: time.Now().Unix(),
		})
	}

	notebook.CellMetadata = new_cell_md

	err = db.SetSubject(self.config, notebook_path_manager.Path(), notebook)
	if err != nil {
		return nil, err
	}

	// Start off with some empty lines.
	if in.Input == "" {
		in.Input = "\n\n\n\n\n\n"
	}

	// Create the new cell with fresh content.
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:      in.Input,
		NotebookId: in.NotebookId,
		CellId:     notebook.LatestCellId,
		Type:       in.Type,

		// New cells are opened for editing.
		CurrentlyEditing: true,
	}

	_, err = self.UpdateNotebookCell(ctx, new_cell_request)
	return notebook, err
}

func (self *ApiServer) UpdateNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	old_notebook := &api_proto.NotebookMetadata{}
	notebook_path_manager := reporting.NewNotebookPathManager(in.NotebookId)
	err = db.GetSubject(self.config, notebook_path_manager.Path(), old_notebook)
	if err != nil {
		return nil, err
	}

	// If the notebook is not properly shared with the user they
	// may not edit it.
	if !reporting.CheckNotebookAccess(old_notebook, user_record.Name) {
		return nil, errors.New("Notebook is not shared with user.")
	}

	if old_notebook.ModifiedTime != in.ModifiedTime {
		return nil, errors.New("Edit clash detected.")
	}

	in.ModifiedTime = time.Now().Unix()

	err = db.SetSubject(self.config, notebook_path_manager.Path(), in)
	if err != nil {
		return nil, err
	}

	// Now also update the indexes.
	err = reporting.UpdateShareIndex(self.config, in)
	return in, err
}

func (self *ApiServer) GetNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to read notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// Check the user is allowed to manipulate this notebook.
	notebook_path_manager := reporting.NewNotebookPathManager(in.NotebookId)

	notebook_metadata := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config,
		notebook_path_manager.Path(), notebook_metadata)
	if err != nil {
		return nil, err
	}

	if !reporting.CheckNotebookAccess(notebook_metadata, user_record.Name) {
		return nil, errors.New("Notebook is not shared with user.")
	}

	notebook := &api_proto.NotebookCell{}
	err = db.GetSubject(self.config,
		notebook_path_manager.Cell(in.CellId).Path(),
		notebook)

	// Cell does not exist, make it a default cell.
	if err == io.EOF {
		notebook = &api_proto.NotebookCell{
			Input:  "",
			Output: "",
			Data:   "{}",
			CellId: notebook.CellId,
			Type:   "Markdown",
		}

		// And store it for next time.
		err = db.SetSubject(self.config,
			notebook_path_manager.Cell(in.CellId).Path(),
			notebook)
		if err != nil {
			return nil, err
		}

	} else if err != nil {
		return nil, err
	}

	return notebook, nil
}

func (self *ApiServer) UpdateNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		Output:           `<div class="padded"><i class="fa fa-spinner fa-spin fa-fw"></i> Calculating...</div>`,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
		Calculating:      true,
	}

	db, _ := datastore.GetDB(self.config)

	// Check that the user has access to this notebook.
	notebook_path_manager := reporting.NewNotebookPathManager(in.NotebookId)
	notebook_metadata := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config,
		notebook_path_manager.Path(), notebook_metadata)
	if err != nil {
		return nil, err
	}

	if !reporting.CheckNotebookAccess(notebook_metadata, user_record.Name) {
		return nil, errors.New("Notebook is not shared with user.")
	}

	// And store it for next time.

	err = db.SetSubject(self.config,
		notebook_path_manager.Cell(in.CellId).Path(),
		notebook_cell)
	if err != nil {
		return nil, err
	}

	// Run the actual query independently.
	query_ctx, query_cancel := context.WithCancel(context.Background())

	acl_manager := vql_subsystem.NewServerACLManager(self.config, user_name)

	global_repo, err := artifacts.GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}

	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config, query_ctx, nil, acl_manager, global_repo,
		notebook_path_manager.Cell(in.CellId),
		"Server.Internal.ArtifactDescription")
	if err != nil {
		return nil, err
	}

	input := in.Input
	cell_type := in.Type

	// For artifacts we parse and check the artifact in the main
	// thread so we can return errors to the user
	// immediately. Ultimately the artifact will be converted to a
	// VQL query to run which we pass to the template engine
	// asynchronously.
	if in.Type == "Artifact" {
		// New artifacts are added to a temporary repository
		// so they do not affect the global one.
		repository := global_repo.Copy()
		artifact_obj, err := repository.LoadYaml(input, true /* validate */)
		if err != nil {
			return nil, err
		}

		// Update the artifact plugin in the template.
		artifact_plugin := artifacts.NewArtifactRepositoryPlugin(repository)
		tmpl.Env.Set("Artifact", artifact_plugin)

		input = fmt.Sprintf(`{{ Query "SELECT * FROM Artifact.%v()" | Table}}`,
			artifact_obj.Name)
	}

	// Update the content asynchronously
	start_time := time.Now()

	// RPC call deadline - if we can complete within 1 second pass
	// the response directly to the RPC caller.
	sub_ctx, sub_cancel := context.WithTimeout(ctx, time.Second)

	// Main error will be delivered to the RPC caller if we can
	// complete the entire operation before the deadline.
	var main_err error

	// Watcher thread: Wait for cancellation from the GUI or a 10 min timeout.
	go func() {
		defer query_cancel()

		cancel_notify, remove_notification := services.ListenForNotification(in.CellId)
		defer remove_notification()

		select {
		// Query is done - get out of here.
		case <-query_ctx.Done():

		// Active cancellation from the GUI.
		case <-cancel_notify:
			tmpl.Scope.Log("Cancelled after %v !", time.Now().Sub(start_time))

			// Set a timeout.
		case <-time.After(10 * time.Minute):
			tmpl.Scope.Log("Query timed out after %v !", time.Now().Sub(start_time))
		}

	}()

	// Main worker: Just run the query until done.
	go func() {
		// Cancel and release the main thread if we
		// finish quickly before the timeout.
		defer sub_cancel()

		// Make sure to cancel the query context if we
		// finished early - the Waiter goroutine above will be
		// released.
		defer query_cancel()

		resp, err := updateCellContents(query_ctx, self.config, tmpl,
			in.CurrentlyEditing, in.NotebookId,
			in.CellId, cell_type, input, in.Input)
		if err != nil {
			main_err = err
			logger := logging.GetLogger(self.config, &logging.GUIComponent)
			logger.Error("Rendering error", err)
		}

		// Update the response if we can.
		notebook_cell = resp
	}()

	// Wait here up to 1 second for immediate response - but if
	// the response takes too long, just give up and return a
	// continuation. The GUI will continue polling for notebook
	// state and will pick up the changes by itself.
	select {
	case <-sub_ctx.Done():
	}

	return notebook_cell, main_err
}

func (self *ApiServer) CancelNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*empty.Empty, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	return &empty.Empty{}, services.NotifyListener(self.config, in.CellId)
}

func (self *ApiServer) UploadNotebookAttachment(
	ctx context.Context,
	in *api_proto.NotebookFileUploadRequest) (*api_proto.NotebookFileUploadResponse, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.NOTEBOOK_EDITOR
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to edit notebooks.")
	}

	decoded, err := base64.StdEncoding.DecodeString(in.Data)
	if err != nil {
		return nil, err
	}

	filename := NewNotebookAttachmentId() + in.Filename
	full_path := path.Join("/notebooks", in.NotebookId,
		string(datastore.SanitizeString(filename)))
	file_store_factory := file_store.GetFileStore(self.config)
	fd, err := file_store_factory.WriteFile(full_path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	_, err = fd.Write(decoded)
	if err != nil {
		return nil, err
	}

	result := &api_proto.NotebookFileUploadResponse{
		Url: full_path,
	}
	return result, nil
}

func (self *ApiServer) CreateNotebookDownloadFile(
	ctx context.Context,
	in *api_proto.NotebookExportRequest) (*empty.Empty, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	user_record, err := users.GetUser(self.config, user_name)
	if err != nil {
		return nil, err
	}

	permissions := acls.PREPARE_RESULTS
	perm, err := acls.CheckAccess(self.config, user_record.Name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to export notebooks.")
	}

	switch in.Type {
	case "zip":
		return &empty.Empty{}, exportZipNotebook(
			self.config, in.NotebookId, user_record.Name)
	default:
		return &empty.Empty{}, exportHTMLNotebook(
			self.config, in.NotebookId, user_record.Name)
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
	notebook_path_manager := reporting.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	if !reporting.CheckNotebookAccess(notebook, principal) {
		return errors.New("Notebook is not shared with user.")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	filename := notebook_path_manager.ZipExport()

	lock_file, err := file_store_factory.WriteFile(filename + ".lock")
	if err != nil {
		return err
	}
	lock_file.Close()

	// Allow 1 hour to export the notebook.
	sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	go func() {
		defer file_store_factory.Delete(filename + ".lock")
		defer cancel()

		err := reporting.ExportNotebookToZip(
			sub_ctx, config_obj, notebook_path_manager)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"export_file": filename,
				"error":       err,
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
	notebook_path_manager := reporting.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	if !reporting.CheckNotebookAccess(notebook, principal) {
		return errors.New("Notebook is not shared with user.")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	filename := notebook_path_manager.HtmlExport()

	lock_file, err := file_store_factory.WriteFile(filename + ".lock")
	if err != nil {
		return err
	}
	lock_file.Close()

	writer, err := file_store_factory.WriteFile(filename)
	if err != nil {
		return err
	}

	// Allow 1 hour to export the notebook.
	sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	go func() {
		defer file_store_factory.Delete(filename + ".lock")
		defer writer.Close()
		defer cancel()

		err := reporting.ExportNotebookToHTML(
			sub_ctx, config_obj, notebook.NotebookId, writer)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"export_file": filename,
				"error":       err,
			}).Error("CreateNotebookDownloadFile")
			return
		}
	}()

	return nil
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_path string) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_path)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			if item.Name() == name+".lock" {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		if strings.HasSuffix(item.Name(), ".lock") {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Path:     path.Join(download_path, item.Name()),
			Size:     uint64(item.Size()),
			Date:     fmt.Sprintf("%v", item.ModTime()),
			Complete: is_complete(item.Name()),
		})
	}

	return result, nil
}

func updateCellContents(
	ctx context.Context,
	config_obj *config_proto.Config,
	tmpl *reporting.GuiTemplateEngine,
	currently_editing bool,
	notebook_id, cell_id, cell_type string,
	input, original_input string) (*api_proto.NotebookCell, error) {

	// Do not let exceptions take down the server.
	defer utils.RecoverVQL(tmpl.Scope)

	output := ""
	var err error

	switch cell_type {

	case "Markdown", "Artifact":
		output, err = tmpl.Execute(
			&artifacts_proto.Report{Template: input})
		if err != nil {
			return nil, err
		}

	case "VQL":
		if input != "" {
			rows := tmpl.Query(input)
			output_any, ok := tmpl.Table(rows).(string)
			if ok {
				output = output_any
			}
		}

	default:
		return nil, errors.New("Unsupported cell type")
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	encoded_data, err := json.Marshal(tmpl.Data)
	if err != nil {
		return nil, err
	}

	notebook_cell := &api_proto.NotebookCell{
		Input:            original_input,
		Output:           output,
		Data:             string(encoded_data),
		Messages:         tmpl.Messages(),
		CellId:           cell_id,
		Type:             cell_type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: currently_editing,
	}

	// And store it for next time.
	notebook_path_manager := reporting.NewNotebookPathManager(notebook_id)
	err = db.SetSubject(config_obj,
		notebook_path_manager.Cell(cell_id).Path(),
		notebook_cell)
	if err != nil {
		return nil, err
	}

	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return nil, err
	}

	new_cell_md := []*api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == cell_id {
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    cell_id,
				Timestamp: time.Now().Unix(),
			})
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}
	notebook.CellMetadata = new_cell_md

	err = db.SetSubject(config_obj, notebook_path_manager.Path(), notebook)
	return notebook_cell, err
}
