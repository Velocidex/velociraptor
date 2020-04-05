package api

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	users "www.velocidex.com/golang/velociraptor/users"
)

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
		return nil, errors.New("User is not allowed to read notebooks.")
	}

	result := &api_proto.Notebooks{}
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// We want a single notebook metadata.
	if in.NotebookId != "" {
		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(self.config,
			reporting.GetNotebookPath(in.NotebookId),
			notebook)
		if err != nil {
			logging.GetLogger(
				self.config, &logging.FrontendComponent).
				Error("Unable to open notebook", err)
			return nil, err
		}

		notebook.AvailableDownloads, err = getAvailableDownloadFiles(self.config,
			path.Join("/downloads/notebooks/", in.NotebookId))
		if err != nil {
			logger := logging.GetLogger(self.config, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"error":       err,
			}).Error("GetNotebooks")
		}

		result.Items = append(result.Items, notebook)

		return result, nil
	}

	// List all available notebooks
	notebook_urns, err := db.ListChildren(
		self.config,
		path.Dir(reporting.GetNotebookPath("X")),
		in.Offset, in.Count)
	if err != nil {
		return nil, err
	}

	for idx, urn := range notebook_urns {
		if uint64(idx) < in.Offset {
			continue
		}

		if uint64(idx) > in.Offset+in.Count {
			break
		}

		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(self.config, urn, notebook)
		if err != nil {
			logging.GetLogger(
				self.config, &logging.FrontendComponent).
				Error("Unable to open notebook", err)
			continue
		}

		if !notebook.Hidden && notebook.NotebookId != "" {
			result.Items = append(result.Items, notebook)
		}
	}

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
		return nil, errors.New("User is not allowed to create notebooks.")
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

	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), in)

	// Add a new single cell to the notebook.
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:            fmt.Sprintf("# %s\n\n%s\n", in.Name, in.Description),
		NotebookId:       in.NotebookId,
		CellId:           new_cell_id,
		Type:             "Markdown",
		CurrentlyEditing: true,
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
		return nil, errors.New("User is not allowed to edit notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), notebook)
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

	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), notebook)
	if err != nil {
		return nil, err
	}

	// Create the new cell with fresh content.
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:            in.Input,
		NotebookId:       in.NotebookId,
		CellId:           notebook.LatestCellId,
		Type:             in.Type,
		CurrentlyEditing: in.CurrentlyEditing,
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
		return nil, errors.New("User is not allowed to edit notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	old_notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), old_notebook)
	if err != nil {
		return nil, err
	}

	if old_notebook.ModifiedTime != in.ModifiedTime {
		return nil, errors.New("Edit clash detected.")
	}

	in.ModifiedTime = time.Now().Unix()

	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), in)

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
		return nil, errors.New("User is not allowed to read notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	notebook := &api_proto.NotebookCell{}
	err = db.GetSubject(self.config,
		reporting.GetNotebookCellPath(in.NotebookId, in.CellId),
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
			reporting.GetNotebookCellPath(in.NotebookId, in.CellId),
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
		return nil, errors.New("User is not allowed to edit notebooks.")
	}

	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config, ctx, user_name, /* principal */
		"Server.Internal.ArtifactDescription")
	if err != nil {
		return nil, err
	}

	output := ""

	switch in.Type {
	case "Markdown":
		output, err = tmpl.Execute(in.Input)
		if err != nil {
			return nil, err
		}

	case "VQL":
		if in.Input != "" {
			rows := tmpl.Query(in.Input)
			output_any, ok := tmpl.Table(rows).(string)
			if ok {
				output = output_any
			}
		}

	default:
		return nil, errors.New("Unsupported cell type")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	encoded_data, err := json.Marshal(tmpl.Data)
	if err != nil {
		return nil, err
	}

	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		Output:           output,
		Data:             string(encoded_data),
		Messages:         *tmpl.Messages,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
	}

	// And store it for next time.
	err = db.SetSubject(self.config,
		reporting.GetNotebookCellPath(in.NotebookId, in.CellId),
		notebook_cell)

	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config,
		reporting.GetNotebookPath(in.NotebookId),
		notebook)
	if err != nil {
		return nil, err
	}

	new_cell_md := []*api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    in.CellId,
				Timestamp: time.Now().Unix(),
			})
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}
	notebook.CellMetadata = new_cell_md

	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), notebook)

	return notebook_cell, err
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
		return nil, errors.New("User is not allowed to edit notebooks.")
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
		return nil, errors.New("User is not allowed to edit notebooks.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(self.config,
		reporting.GetNotebookPath(in.NotebookId),
		notebook)
	if err != nil {
		return nil, err
	}

	file_store_factory := file_store.GetFileStore(self.config)
	filename := path.Join("/downloads/notebooks", notebook.NotebookId,
		fmt.Sprintf("%s-%s.html", notebook.NotebookId,
			time.Now().Format("20060102150405Z")))

	lock_file, err := file_store_factory.WriteFile(filename + ".lock")
	if err != nil {
		return nil, err
	}
	lock_file.Close()

	writer, err := file_store_factory.WriteFile(filename)
	if err != nil {
		return nil, err
	}

	go func() {
		defer file_store_factory.Delete(filename + ".lock")
		defer writer.Close()

		err := reporting.ExportNotebookToHTML(
			self.config, notebook.NotebookId, writer)
		if err != nil {
			logger := logging.GetLogger(self.config, &logging.GUIComponent)
			logger.WithFields(logrus.Fields{
				"notebook_id": notebook.NotebookId,
				"export_file": filename,
				"error":       err,
			}).Error("CreateNotebookDownloadFile")
			return
		}
	}()

	return &empty.Empty{}, err
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
