package api

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	context "golang.org/x/net/context"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	users "www.velocidex.com/golang/velociraptor/users"
)

func (self *ApiServer) GetNotebooks(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.Notebooks, error) {

	result := &api_proto.Notebooks{}
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	notebook_urns, err := db.ListChildren(
		self.config,
		path.Dir(reporting.GetNotebookPath("X")),
		in.Offset, in.Count)
	if err != nil {
		return nil, err
	}

	for _, urn := range notebook_urns {
		noteboook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(self.config, urn, noteboook)
		if err != nil {
			logging.GetLogger(
				self.config, &logging.FrontendComponent).
				Error("Unable to open notebook", err)
			continue
		}

		result.Items = append(result.Items, noteboook)
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

	user := GetGRPCUserInfo(self.config, ctx).Name

	// If user is not found then reject it.
	user_record, err := users.GetUser(self.config, user)
	if err != nil {
		return nil, err
	}

	if user_record.ReadOnly {
		return nil, errors.New("User is not allowed to create notebooks.")
	}
	in.Creator = user
	in.CreatedTime = time.Now().Unix()
	in.ModifiedTime = in.CreatedTime
	in.NotebookId = NewNotebookId()

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}
	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), in)

	return &empty.Empty{}, err
}

func (self *ApiServer) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookMetadata, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user := GetGRPCUserInfo(self.config, ctx).Name

	// If user is not found then reject it.
	user_record, err := users.GetUser(self.config, user)
	if err != nil {
		return nil, err
	}

	if user_record.ReadOnly {
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

	new_cells := []string{}
	added := false

	notebook.LatestCellId = NewNotebookCellId()

	for _, cell_id := range notebook.Cells {
		if cell_id == in.CellId {
			new_cells = append(new_cells, cell_id)
			new_cells = append(new_cells, notebook.LatestCellId)
			added = true
			continue
		}
		new_cells = append(new_cells, cell_id)
	}

	// Add it to the end of the document.
	if !added {
		new_cells = append(new_cells, notebook.LatestCellId)
	}

	notebook.Cells = new_cells

	err = db.SetSubject(self.config, reporting.GetNotebookPath(
		in.NotebookId), notebook)

	return notebook, err
}

func (self *ApiServer) UpdateNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user := GetGRPCUserInfo(self.config, ctx).Name

	// If user is not found then reject it.
	user_record, err := users.GetUser(self.config, user)
	if err != nil {
		return nil, err
	}

	if user_record.ReadOnly {
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

	user := GetGRPCUserInfo(self.config, ctx).Name

	// If user is not found then reject it.
	user_record, err := users.GetUser(self.config, user)
	if err != nil {
		return nil, err
	}

	if user_record.ReadOnly {
		return nil, errors.New("User is not allowed to edit notebooks.")
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

	tmpl, err := reporting.NewGuiTemplateEngine(
		self.config, ctx, "Server.Internal.ArtifactDescription")
	if err != nil {
		return nil, err
	}

	output, err := tmpl.Execute(in.Input)
	if err != nil {
		return nil, err
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	encoded_data, err := json.Marshal(tmpl.Data)
	if err != nil {
		return nil, err
	}

	notebook := &api_proto.NotebookCell{
		Input:     in.Input,
		Output:    output,
		Data:      string(encoded_data),
		Messages:  *tmpl.Messages,
		CellId:    in.CellId,
		Timestamp: time.Now().Unix(),
	}

	// And store it for next time.
	err = db.SetSubject(self.config,
		reporting.GetNotebookCellPath(in.NotebookId, notebook.CellId),
		notebook)
	return notebook, err
}
