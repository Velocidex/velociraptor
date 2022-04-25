package api

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"

	errors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	file_store "www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/flows"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func (self *ApiServer) ExportNotebook(
	ctx context.Context,
	in *api_proto.NotebookExportRequest) (*emptypb.Empty, error) {
	return nil, errors.New("not implementated")
}

// Get all the current user's notebooks and those notebooks shared
// with them.
func (self *ApiServer) GetNotebooks(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.Notebooks, error) {

	defer Instrument("GetNotebooks")()

	// Empty creators are called internally.
	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
		notebook_path_manager := paths.NewNotebookPathManager(
			in.NotebookId)
		notebook := &api_proto.NotebookMetadata{}
		err := db.GetSubject(self.config, notebook_path_manager.Path(),
			notebook)

		// Handle the EOF especially: it means there is no such
		// notebook and return an empty result set.
		if errors.Is(err, os.ErrNotExist) || notebook.NotebookId == "" {
			return result, nil
		}
		if err != nil {
			logging.GetLogger(
				self.config, &logging.FrontendComponent).
				Error("Unable to open notebook: %v", err)
			return nil, err
		}

		// An error here just means there are no AvailableDownloads.
		notebook.AvailableDownloads, _ = getAvailableDownloadFiles(self.config,
			notebook_path_manager.HtmlExport().Dir())

		if in.IncludeUploads {
			notebook.AvailableUploads, _ = getAvailableUploadFiles(self.config,
				notebook_path_manager)
		}

		notebook.Timelines = getAvailableTimelines(
			self.config, notebook_path_manager)

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
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "N." + result
}

func NewNotebookAttachmentId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NA." + result
}

func NewNotebookCellId() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return "NC." + result
}

func (self *ApiServer) NewNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	defer Instrument("NewNotebook")()

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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

	// Allow hunt notebooks to be created with a specified hunt ID.
	if !strings.HasPrefix(in.NotebookId, "N.H.") &&
		!strings.HasPrefix(in.NotebookId, "N.F.") &&
		!strings.HasPrefix(in.NotebookId, "N.E.") {
		in.NotebookId = NewNotebookId()
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// Store the notebook metadata first before creating the
	// cells. Calculating the cells will try to open the notebook.
	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)
	err = db.SetSubject(self.config, notebook_path_manager.Path(), in)
	if err != nil {
		return nil, err
	}

	err = self.createInitialNotebook(ctx, user_name, in)
	if err != nil {
		return nil, err
	}

	// Add the new notebook to the index so it can be seen. Only
	// non-hunt notebooks are searchable in the index since the
	// hunt notebooks are always found in the hunt results.
	err = reporting.UpdateShareIndex(self.config, in)
	if err != nil {
		return nil, err
	}

	err = db.SetSubject(self.config, notebook_path_manager.Path(), in)
	return in, err
}

// Create the initial cells of the notebook.
func (self *ApiServer) createInitialNotebook(
	ctx context.Context,
	user_name string,
	notebook_metadata *api_proto.NotebookMetadata) error {

	// All cells receive a header from the name and description of
	// the notebook.
	new_cells := []*api_proto.NotebookCellRequest{{
		Input: fmt.Sprintf("# %s\n\n%s\n", notebook_metadata.Name,
			notebook_metadata.Description),
		Type:             "Markdown",
		CurrentlyEditing: true,
	}}

	// Figure out what type of content to create depending on the type
	// of the notebook
	if notebook_metadata.Context != nil {
		if notebook_metadata.Context.HuntId != "" {
			new_cells = getCellsForHunt(ctx, self.config,
				notebook_metadata.Context.HuntId, notebook_metadata)
		} else if notebook_metadata.Context.FlowId != "" &&
			notebook_metadata.Context.ClientId != "" {
			new_cells = getCellsForFlow(ctx, self.config,
				notebook_metadata.Context.ClientId,
				notebook_metadata.Context.FlowId, notebook_metadata)
		} else if notebook_metadata.Context.EventArtifact != "" &&
			notebook_metadata.Context.ClientId != "" {
			new_cells = getCellsForEvents(ctx, self.config,
				notebook_metadata.Context.ClientId,
				notebook_metadata.Context.EventArtifact, notebook_metadata)
		}
	}

	for _, cell := range new_cells {
		new_cell_id := NewNotebookCellId()

		notebook_metadata.CellMetadata = append(notebook_metadata.CellMetadata, &api_proto.NotebookCell{
			CellId:    new_cell_id,
			Env:       cell.Env,
			Timestamp: time.Now().Unix(),
		})
		cell.NotebookId = notebook_metadata.NotebookId
		cell.CellId = new_cell_id

		_, err := self.updateNotebookCell(ctx, notebook_metadata, user_name, cell)
		if err != nil {
			return err
		}
	}
	return nil
}

func getCellsForEvents(ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, artifact_name string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil
	}

	result := getCustomCells(config_obj, repository,
		artifact_name, notebook_metadata)

	// If there are no custom cells, add the default cell.
	if len(result) == 0 {
		// Start the event display 1 day ago.
		start_time := time.Now().AddDate(0, 0, -1).UTC().Format(time.RFC3339)
		if notebook_metadata.Context.StartTime > 0 {
			start_time = utils.ParseTimeFromInt64(
				notebook_metadata.Context.StartTime).UTC().Format(time.RFC3339)
		}

		end_time := time.Now().UTC().Format(time.RFC3339)
		if notebook_metadata.Context.EndTime > 0 {
			end_time = utils.ParseTimeFromInt64(
				notebook_metadata.Context.EndTime).UTC().Format(time.RFC3339)
		}

		result = append(result, &api_proto.NotebookCellRequest{
			Type: "VQL",

			// This env dict overlays on top of the global
			// notebook env where we can find hunt_id, flow_id
			// etc.
			Env: []*api_proto.Env{{
				Key: "ArtifactName", Value: artifact_name,
			}},
			Input: fmt.Sprintf(`/*
# Events from %v
*/
LET StartTime <= "%s"
LET EndTime <= "%s"

SELECT *, timestamp(epoch=_ts) AS ServerTime
 FROM source(start_time=StartTime, end_time=EndTime)
LIMIT 50
`, artifact_name, start_time, end_time),
		})
	}

	return result
}

func getCustomCells(
	config_obj *config_proto.Config,
	repository services.Repository,
	source string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {
	var result []*api_proto.NotebookCellRequest

	// Check if the artifact has custom notebook cells defined.
	artifact_source, pres := repository.GetSource(config_obj, source)
	if !pres {
		return nil
	}
	env := []*api_proto.Env{{
		Key: "ArtifactName", Value: source,
	}}

	// If the artifact_source defines a notebook, let it do its own thing.
	for _, cell := range artifact_source.Notebook {
		for _, i := range cell.Env {
			env = append(env, &api_proto.Env{
				Key:   i.Key,
				Value: i.Value,
			})
		}

		request := &api_proto.NotebookCellRequest{
			Type:  cell.Type,
			Env:   env,
			Input: cell.Template}

		switch strings.ToLower(cell.Type) {
		case "vql", "md", "markdown":
			result = append(result, request)

		case "vql_suggestion":
			request.Type = "vql"
			request.Name = cell.Name
			notebook_metadata.Suggestions = append(
				notebook_metadata.Suggestions, request)

		default:
			logger := logging.GetLogger(config_obj, &logging.GUIComponent)
			logger.Error("getDefaultCellsForSources: Cell type %v invalid",
				cell.Type)
		}
	}
	return result
}

func getCellsForHunt(ctx context.Context,
	config_obj *config_proto.Config,
	hunt_id string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {

	dispatcher := services.GetHuntDispatcher()
	if dispatcher == nil {
		return nil
	}

	hunt_obj, pres := dispatcher.GetHunt(hunt_id)
	if !pres {
		return nil
	}
	sources := hunt_obj.ArtifactSources
	if len(sources) == 0 {
		if hunt_obj.StartRequest != nil {
			sources = hunt_obj.StartRequest.Artifacts
		} else {
			return nil
		}
	}

	// Add a default hunt suggestion
	notebook_metadata.Suggestions = append(notebook_metadata.Suggestions,
		&api_proto.NotebookCellRequest{
			Name: "Hunt Progress",
			Type: "vql",
			Input: `

LET ColumnTypes <= dict(
   ClientId="client_id",
   FlowId="flow",
   StartedTime="timestamp"
)

/*
# Flows with ERROR status
*/
SELECT ClientId, FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_collected_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'ERROR'

/*
## Flows with RUNNING status
*/
SELECT ClientId, FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_collected_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'RUNNING'

/*
## Flows with FINISHED status
*/
SELECT ClientId, FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_collected_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'Finished'
`})

	return getDefaultCellsForSources(config_obj, sources, notebook_metadata)
}

func getCellsForFlow(ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {

	flow_context, err := flows.LoadCollectionContext(config_obj, client_id, flow_id)
	if err != nil {
		return nil
	}

	sources := flow_context.ArtifactsWithResults
	if len(sources) == 0 && flow_context.Request != nil {
		sources = flow_context.Request.Artifacts
	}

	return getDefaultCellsForSources(config_obj, sources, notebook_metadata)
}

func getDefaultCellsForSources(
	config_obj *config_proto.Config,
	sources []string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil
	}

	// Create one table per artifact by default.
	var result []*api_proto.NotebookCellRequest

	for _, source := range sources {
		artifact, pres := repository.Get(config_obj, source)
		if pres {
			notebook_metadata.ColumnTypes = append(notebook_metadata.ColumnTypes,
				artifact.ColumnTypes...)
		}

		new_cells := getCustomCells(config_obj, repository,
			source, notebook_metadata)
		result = append(result, new_cells...)

		// Build a default empty notebook that shows off all the
		// results if there are no custom cells.
		if len(new_cells) == 0 {
			result = append(result, &api_proto.NotebookCellRequest{
				Type: "VQL",

				// This env dict overlays on top of the global
				// notebook env where we can find hunt_id, flow_id
				// etc.
				Env: []*api_proto.Env{{
					Key: "ArtifactName", Value: source,
				}},
				Input: fmt.Sprintf(`
/*
# %v
*/
SELECT * FROM source(artifact=%q)
LIMIT 50
`, source, source),
			})
		}
	}

	return result
}

func (self *ApiServer) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookMetadata, error) {

	defer Instrument("NewNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)
	err = db.GetSubject(self.config, notebook_path_manager.Path(), notebook)
	if err != nil {
		return nil, err
	}

	new_cell_md := []*api_proto.NotebookCell{}
	added := false

	notebook.LatestCellId = NewNotebookCellId()

	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {
			// New cell goes above existing cell.
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    notebook.LatestCellId,
				Timestamp: time.Now().Unix(),
			})
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    cell_md.CellId,
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
		Env:        in.Env,

		// New cells are opened for editing.
		CurrentlyEditing: true,
	}

	_, err = self.UpdateNotebookCell(ctx, new_cell_request)
	return notebook, err
}

func (self *ApiServer) UpdateNotebook(
	ctx context.Context,
	in *api_proto.NotebookMetadata) (*api_proto.NotebookMetadata, error) {

	defer Instrument("UpdateNotebook")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)
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

	defer Instrument("GetNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)

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
	if errors.Is(err, os.ErrNotExist) {
		return &api_proto.NotebookCell{
			Input:  "",
			Output: "",
			Data:   "{}",
			CellId: in.CellId,
			Type:   "Markdown",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return notebook, nil
}

func (self *ApiServer) UpdateNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	defer Instrument("UpdateNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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

	// Check that the user has access to this notebook.
	notebook_path_manager := paths.NewNotebookPathManager(in.NotebookId)
	notebook_metadata := &api_proto.NotebookMetadata{}
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(self.config,
		notebook_path_manager.Path(), notebook_metadata)
	if err != nil {
		return nil, err
	}

	if !reporting.CheckNotebookAccess(notebook_metadata, user_record.Name) {
		return nil, errors.New("Notebook is not shared with user.")
	}

	return self.updateNotebookCell(ctx, notebook_metadata, user_name, in)
}

func (self *ApiServer) updateNotebookCell(
	ctx context.Context,
	notebook_metadata *api_proto.NotebookMetadata,
	user_name string,
	in *api_proto.NotebookCellRequest) (*api_proto.NotebookCell, error) {

	notebook_cell := &api_proto.NotebookCell{
		Input:            in.Input,
		Output:           `<div class="padded"><i class="fa fa-spinner fa-spin fa-fw"></i> Calculating...</div>`,
		CellId:           in.CellId,
		Type:             in.Type,
		Timestamp:        time.Now().Unix(),
		CurrentlyEditing: in.CurrentlyEditing,
		Calculating:      true,
		Env:              in.Env,
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	// And store it for next time.
	notebook_path_manager := paths.NewNotebookPathManager(
		notebook_metadata.NotebookId)
	err = db.SetSubject(self.config,
		notebook_path_manager.Cell(in.CellId).Path(),
		notebook_cell)
	if err != nil {
		return nil, err
	}

	// Run the actual query independently.
	query_ctx, query_cancel := context.WithCancel(context.Background())

	acl_manager := vql_subsystem.NewServerACLManager(self.config, user_name)

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}
	global_repo, err := manager.GetGlobalRepository(self.config)
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

	tmpl.SetEnv("NotebookId", in.NotebookId)

	// Register a progress reporter so we can monitor how the
	// template rendering is going.
	tmpl.Progress = &progressReporter{
		config_obj:    self.config,
		notebook_cell: notebook_cell,
		notebook_id:   in.NotebookId,
		start:         time.Now(),
	}

	// Add the notebook environment into the cell template.
	for _, env := range notebook_metadata.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	// Also apply the cell env
	for _, env := range in.Env {
		tmpl.SetEnv(env.Key, env.Value)
	}

	input := in.Input
	cell_type := in.Type

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

		cancel_notify, remove_notification := services.GetNotifier().
			ListenForNotification(in.CellId)
		defer remove_notification()

		default_notebook_expiry := self.config.Defaults.NotebookCellTimeoutMin
		if default_notebook_expiry == 0 {
			default_notebook_expiry = 10
		}

		select {
		// Query is done - get out of here.
		case <-query_ctx.Done():

		// Active cancellation from the GUI.
		case <-cancel_notify:
			tmpl.Scope.Log("Cancelled after %v !", time.Since(start_time))

			// Set a timeout.
		case <-time.After(time.Duration(default_notebook_expiry) * time.Minute):
			tmpl.Scope.Log("Query timed out after %v !", time.Since(start_time))
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

		// Close the template when we are done with it.
		defer tmpl.Close()

		resp, err := updateCellContents(query_ctx, self.config, tmpl,
			in.CurrentlyEditing, in.NotebookId,
			in.CellId, cell_type, in.Env, input, in.Input)
		if err != nil {
			main_err = err
			logger := logging.GetLogger(self.config, &logging.GUIComponent)
			logger.Error("Rendering error: %v", err)
		}

		// Update the response if we can.
		if resp != nil {
			notebook_cell = resp
		}
	}()

	// Wait here up to 1 second for immediate response - but if
	// the response takes too long, just give up and return a
	// continuation. The GUI will continue polling for notebook
	// state and will pick up the changes by itself.
	<-sub_ctx.Done()

	return notebook_cell, main_err
}

func (self *ApiServer) CancelNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest) (*emptypb.Empty, error) {

	defer Instrument("CancelNotebookCell")()

	if !strings.HasPrefix(in.NotebookId, "N.") {
		return nil, errors.New("Invalid NoteboookId")
	}

	if !strings.HasPrefix(in.CellId, "NC.") {
		return nil, errors.New("Invalid NoteboookCellId")
	}

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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

	// Unset the calculating bit in the notebook in case the
	// renderer is not actually running (e.g. server restart).
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}
	notebook_cell_path_manager := paths.NewNotebookPathManager(
		in.NotebookId).Cell(in.CellId)
	notebook_cell := &api_proto.NotebookCell{}
	err = db.GetSubject(self.config, notebook_cell_path_manager.Path(),
		notebook_cell)
	if err != nil || notebook_cell.CellId != in.CellId {
		return nil, errors.New("No such cell")
	}

	notebook_cell.Calculating = false
	// Make sure we write the cancel message ASAP
	err = db.SetSubject(self.config, notebook_cell_path_manager.Path(),
		notebook_cell)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, services.GetNotifier().NotifyListener(
		self.config, in.CellId, "CancelNotebookCell")
}

func (self *ApiServer) UploadNotebookAttachment(
	ctx context.Context,
	in *api_proto.NotebookFileUploadRequest) (*api_proto.NotebookFileUploadResponse, error) {

	defer Instrument("UploadNotebookAttachment")()

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
	full_path := paths.NewNotebookPathManager(in.NotebookId).
		Attachment(filename)
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
		Url: full_path.AsClientPath(),
	}
	return result, nil
}

func (self *ApiServer) CreateNotebookDownloadFile(
	ctx context.Context,
	in *api_proto.NotebookExportRequest) (*emptypb.Empty, error) {

	defer Instrument("CreateNotebookDownloadFile")()

	user_name := GetGRPCUserInfo(self.config, ctx, self.ca_pool).Name
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
		return &emptypb.Empty{}, exportZipNotebook(
			self.config, in.NotebookId, user_record.Name)
	default:
		return &emptypb.Empty{}, exportHTMLNotebook(
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
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	if !reporting.CheckNotebookAccess(notebook, principal) {
		return errors.New("Notebook is not shared with user.")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	filename := notebook_path_manager.ZipExport()
	lock_file_name := filename.SetType(api.PATH_TYPE_FILESTORE_LOCK)

	lock_file, err := file_store_factory.WriteFile(lock_file_name)
	if err != nil {
		return err
	}
	lock_file.Close()

	// Allow 1 hour to export the notebook.
	sub_ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

	go func() {
		defer func() {
			_ = file_store_factory.Delete(lock_file_name)
		}()

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
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	if !reporting.CheckNotebookAccess(notebook, principal) {
		return errors.New("Notebook is not shared with user.")
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	filename := notebook_path_manager.HtmlExport()
	lock_file_name := filename.SetType(api.PATH_TYPE_FILESTORE_LOCK)

	lock_file, err := file_store_factory.WriteFile(lock_file_name)
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
		defer func() { _ = file_store_factory.Delete(lock_file_name) }()
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

func getAvailableTimelines(
	config_obj *config_proto.Config,
	path_manager *paths.NotebookPathManager) []string {

	result := []string{}
	db, err := datastore.GetDB(config_obj)
	files, err := db.ListChildren(config_obj, path_manager.SuperTimelineDir())
	if err != nil {
		return nil
	}

	for _, f := range files {
		if !f.IsDir() {
			result = append(result, f.Base())
		}
	}
	return result
}

func getAvailableDownloadFiles(config_obj *config_proto.Config,
	download_path api.FSPathSpec) (*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(download_path)
	if err != nil {
		return nil, err
	}

	is_complete := func(name string) bool {
		for _, item := range files {
			ps := item.PathSpec()
			// If there is a lock file we are not done.
			if ps.Base() == name &&
				ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
				return false
			}
		}
		return true
	}

	for _, item := range files {
		ps := item.PathSpec()

		// Skip lock files
		if ps.Type() == api.PATH_TYPE_FILESTORE_LOCK {
			continue
		}

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: is_complete(ps.Base()),
		})
	}

	return result, nil
}

func getAvailableUploadFiles(config_obj *config_proto.Config,
	notebook_path_manager *paths.NotebookPathManager) (
	*api_proto.AvailableDownloads, error) {
	result := &api_proto.AvailableDownloads{}

	file_store_factory := file_store.GetFileStore(config_obj)
	files, err := file_store_factory.ListDirectory(
		notebook_path_manager.UploadsDir())
	if err != nil {
		return nil, err
	}

	for _, item := range files {
		ps := item.PathSpec()

		result.Files = append(result.Files, &api_proto.AvailableDownloadFile{
			Name:     item.Name(),
			Type:     api.GetExtensionForFilestore(ps),
			Path:     ps.AsClientPath(),
			Size:     uint64(item.Size()),
			Date:     item.ModTime().UTC().Format(time.RFC3339),
			Complete: true,
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
	env []*api_proto.Env,
	input, original_input string) (res *api_proto.NotebookCell, err error) {

	output := ""

	cell_type = strings.ToLower(cell_type)

	// Create a new cell to set the result in.
	make_cell := func(output string) *api_proto.NotebookCell {
		messages := tmpl.Messages()

		encoded_data, err := json.Marshal(tmpl.Data)
		if err != nil {
			messages = append(messages,
				fmt.Sprintf("Error: %v", err))
		}

		return &api_proto.NotebookCell{
			Input:            original_input,
			Output:           output,
			Data:             string(encoded_data),
			Messages:         tmpl.Messages(),
			CellId:           cell_id,
			Type:             cell_type,
			Env:              env,
			Timestamp:        time.Now().Unix(),
			CurrentlyEditing: currently_editing,
			Duration:         int64(time.Since(tmpl.Start).Seconds()),
		}
	}

	// If an error occurs it is important to ensure the cell is
	// still written with an error message.
	make_error_cell := func(output string, err error) (
		*api_proto.NotebookCell, error) {
		notebook_cell := make_cell(output)
		notebook_cell.Messages = append(notebook_cell.Messages,
			fmt.Sprintf("Error: %v", err))
		setCell(config_obj, notebook_id, notebook_cell)
		return notebook_cell, err
	}

	// Do not let exceptions take down the server.
	defer func() {
		r := recover()
		if r != nil {
			res, err = make_error_cell("", fmt.Errorf("PANIC: %v", r))
		}
	}()

	switch cell_type {

	case "vql_suggestion":
		// noop - these cells will be created by the user on demand.

	case "markdown", "md":
		// A Markdown cell just feeds directly into the
		// template.
		output, err = tmpl.Execute(&artifacts_proto.Report{Template: input})
		if err != nil {
			return make_error_cell(output, err)
		}

	case "vql":
		// A VQL cell gets converted to a set of VQL and
		// markdown fragments.
		cell_content, err := reporting.ConvertVQLCellToContent(input)
		if err != nil {
			// Ignore errors and just treat the whole
			// thing as VQL - this will fail to render the
			// comment and just ignore it - it is probably
			// malformed.
			cell_content = &reporting.Content{}
			cell_content.PushVQL(input)
		}

		for _, fragment := range cell_content.Fragments {
			if fragment.VQL != "" {
				rows := tmpl.Query(fragment.VQL)
				output_any, ok := tmpl.Table(rows).(string)
				if ok {
					output += output_any
				}

			} else if fragment.Comment != "" {
				lines := strings.SplitN(fragment.Comment, "\n", 2)
				if len(lines) <= 1 {
					input = lines[0]
				} else {
					input = lines[1]
				}
				fragment_output, err := tmpl.Execute(&artifacts_proto.Report{Template: input})
				if err != nil {
					return make_error_cell(output, err)
				}
				output += fragment_output
			}
		}

	default:
		return make_error_cell(output, errors.New("Unsupported cell type."))
	}

	tmpl.Close()

	notebook_cell := make_cell(output)
	return notebook_cell, setCell(config_obj, notebook_id, notebook_cell)
}

func setCell(
	config_obj *config_proto.Config,
	notebook_id string,
	notebook_cell *api_proto.NotebookCell) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	// And store it for next time.
	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	err = db.SetSubject(config_obj,
		notebook_path_manager.Cell(notebook_cell.CellId).Path(),
		notebook_cell)
	if err != nil {
		return err
	}

	// Open the notebook and update the cell's timestamp.
	notebook := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook)
	if err != nil {
		return err
	}

	// Update the cell's timestamp so the gui will refresh it.
	new_cell_md := []*api_proto.NotebookCell{}
	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == notebook_cell.CellId {
			new_cell_md = append(new_cell_md, &api_proto.NotebookCell{
				CellId:    notebook_cell.CellId,
				Timestamp: time.Now().Unix(),
			})
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}
	notebook.CellMetadata = new_cell_md

	return db.SetSubject(config_obj, notebook_path_manager.Path(), notebook)
}

type progressReporter struct {
	config_obj            *config_proto.Config
	notebook_cell         *api_proto.NotebookCell
	notebook_id, table_id string
	last, start           time.Time
}

func (self *progressReporter) Report(message string) {
	now := time.Now()
	if now.Before(self.last.Add(4 * time.Second)) {
		return
	}

	self.last = now
	duration := time.Since(self.start).Round(time.Second)

	notebook_cell := proto.Clone(self.notebook_cell).(*api_proto.NotebookCell)
	notebook_cell.Output = fmt.Sprintf(`
<div class="padded"><i class="fa fa-spinner fa-spin fa-fw"></i>
   Calculating...  (%v after %v)
</div>
<div class="panel">
   <grr-csv-viewer base-url="'v1/GetTable'"
                   params='{"notebook_id":"%s","cell_id":"%s","table_id":1,"message": "%s"}' />
</div>
`,
		message, duration,
		self.notebook_id, self.notebook_cell.CellId, message)
	notebook_cell.Timestamp = now.Unix()
	notebook_cell.Duration = int64(duration.Seconds())

	// Cant do anything if we can not set the notebook times
	_ = setCell(self.config_obj, self.notebook_id, notebook_cell)
}
