package notebook

import (
	"context"
	"fmt"
	"strings"
	"time"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *NotebookManager) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest, username string) (
	*api_proto.NotebookMetadata, error) {

	notebook, err := self.store.GetNotebook(in.NotebookId)
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
	err = self.store.SetNotebook(notebook)
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

	_, err = self.UpdateNotebookCell(ctx, notebook, username, new_cell_request)
	return notebook, err
}

// Create the initial cells of the notebook.
func CreateInitialNotebook(ctx context.Context,
	config_obj *config_proto.Config,
	notebook_metadata *api_proto.NotebookMetadata,
	principal string) error {

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
			new_cells = getCellsForHunt(ctx, config_obj,
				notebook_metadata.Context.HuntId, notebook_metadata)
		} else if notebook_metadata.Context.FlowId != "" &&
			notebook_metadata.Context.ClientId != "" {
			new_cells = getCellsForFlow(ctx, config_obj,
				notebook_metadata.Context.ClientId,
				notebook_metadata.Context.FlowId, notebook_metadata)
		} else if notebook_metadata.Context.EventArtifact != "" &&
			notebook_metadata.Context.ClientId != "" {
			new_cells = getCellsForEvents(ctx, config_obj,
				notebook_metadata.Context.ClientId,
				notebook_metadata.Context.EventArtifact, notebook_metadata)
		}
	}

	notebook_manager, err := services.GetNotebookManager()
	if err != nil {
		return err
	}

	for _, cell := range new_cells {
		new_cell_id := NewNotebookCellId()

		notebook_metadata.CellMetadata = append(
			notebook_metadata.CellMetadata, &api_proto.NotebookCell{
				CellId:    new_cell_id,
				Env:       cell.Env,
				Timestamp: time.Now().Unix(),
			})
		cell.NotebookId = notebook_metadata.NotebookId
		cell.CellId = new_cell_id

		_, err := notebook_manager.UpdateNotebookCell(
			ctx, notebook_metadata, principal, cell)
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

	manager, err := services.GetRepositoryManager(config_obj)
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

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil
	}

	flow_details, err := launcher.GetFlowDetails(
		config_obj, client_id, flow_id)
	if err != nil {
		return nil
	}
	flow_context := flow_details.Context

	sources := flow_context.ArtifactsWithResults
	if len(sources) == 0 && flow_context.Request != nil {
		sources = flow_context.Request.Artifacts
	}

	notebook_metadata.Suggestions = append(notebook_metadata.Suggestions,
		&api_proto.NotebookCellRequest{
			Name: "Collection logs",
			Type: "vql",
			Input: `
/*
# Flow logs
*/

SELECT * FROM flow_logs(client_id=ClientId, flow_id=FlowId)
`,
		})

	return getDefaultCellsForSources(config_obj, sources, notebook_metadata)
}

func getDefaultCellsForSources(
	config_obj *config_proto.Config,
	sources []string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {
	manager, err := services.GetRepositoryManager(config_obj)
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
