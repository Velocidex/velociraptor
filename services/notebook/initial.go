package notebook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *NotebookManager) NewNotebookCell(
	ctx context.Context,
	in *api_proto.NotebookCellRequest, username string) (
	*api_proto.NotebookMetadata, error) {

	// Calculate the cell first then insert it into the notebook.
	new_version := GetNextVersion("")
	new_cell_request := &api_proto.NotebookCellRequest{
		Input:             in.Input,
		Output:            in.Output,
		CellId:            NewNotebookCellId(),
		NotebookId:        in.NotebookId,
		Version:           new_version,
		AvailableVersions: []string{new_version},
		Type:              in.Type,
		Env:               in.Env,
		Sync:              in.Sync,

		// New cells are opened for editing.
		CurrentlyEditing: true,
	}

	// TODO: This is not thread safe!
	notebook, err := self.Store.GetNotebook(in.NotebookId)
	if err != nil {
		return nil, err
	}

	// Start off with some empty lines.
	if in.Input == "" {
		in.Input = "\n\n\n\n\n\n"
	}

	new_cell, err := self.UpdateNotebookCell(
		ctx, notebook, username, new_cell_request)
	if err != nil {
		return nil, err
	}

	// The notebook only keep summary metadata and not the full
	// results.
	new_cell_summary := &api_proto.NotebookCell{
		CellId:            new_cell.CellId,
		CurrentVersion:    new_cell.CurrentVersion,
		AvailableVersions: new_cell.AvailableVersions,
		Timestamp:         new_cell.Timestamp,
	}

	added := false
	now := utils.GetTime().Now().Unix()

	new_cell_md := []*api_proto.NotebookCell{}

	for _, cell_md := range notebook.CellMetadata {
		if cell_md.CellId == in.CellId {

			// New cell goes above existing cell.
			new_cell_md = append(new_cell_md, new_cell_summary)

			cell_md.Timestamp = now
			new_cell_md = append(new_cell_md, cell_md)
			added = true
			continue
		}
		new_cell_md = append(new_cell_md, cell_md)
	}

	// Add it to the end of the document.
	if !added {
		new_cell_md = append(new_cell_md, new_cell_summary)
	}

	notebook.LatestCellId = new_cell.CellId
	notebook.CellMetadata = new_cell_md
	notebook.ModifiedTime = new_cell.Timestamp

	err = self.Store.SetNotebook(notebook)
	if err != nil {
		return nil, err
	}

	return notebook, err
}

func getSpec(name string,
	env []*api_proto.Env,
	specs []*flows_proto.ArtifactSpec,
	artifact *artifacts_proto.Artifact) []*api_proto.Env {

	env_dict := ordereddict.NewDict()

	for _, spec := range specs {
		if spec.Artifact == name {
			if spec.Parameters != nil {
				for _, e := range spec.Parameters.Env {
					env_dict.Set(e.Key, e.Value)
				}
			}
		}
	}

	for _, p := range artifact.Parameters {
		value, pres := env_dict.GetString(p.Name)
		if !pres {
			value = p.Default
		}
		env = append(env, &api_proto.Env{
			Key:   p.Name,
			Value: value,
		})
	}

	return env
}

func getInitialCellsFromArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.NotebookMetadata) (
	result []*api_proto.NotebookCellRequest, err error) {

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, err
	}
	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	for _, artifact_name := range in.Artifacts {
		artifact, pres := repository.Get(ctx, config_obj, artifact_name)
		if !pres {
			continue
		}

		for _, s := range artifact.Sources {
			for _, n := range s.Notebook {
				env := []*api_proto.Env{}
				for _, i := range n.Env {
					env = append(env, &api_proto.Env{
						Key:   i.Key,
						Value: i.Value,
					})
				}

				// Add any specs from the template parameters.
				env = getSpec(artifact_name, env, in.Specs, artifact)

				switch strings.ToLower(n.Type) {
				case "none":
					// Means no cell to be produced.
					result = append(result, &api_proto.NotebookCellRequest{
						Type: n.Type,
					})

				case "vql", "md", "markdown":
					result = append(result, &api_proto.NotebookCellRequest{
						Type:   n.Type,
						Input:  n.Template,
						Output: n.Output,
						Env:    env,

						// Need to wait for all cells to calculate or
						// we will overload the netowork workers if
						// there are too many.
						Sync: true,
					})
				case "vql_suggestion":
					in.Suggestions = append(in.Suggestions,
						&api_proto.NotebookCellRequest{
							Type:  "vql",
							Name:  n.Name,
							Input: n.Template,
							Env:   env,
						})
				}
			}
		}
	}
	return result, nil
}

func getInitialCells(
	ctx context.Context,
	config_obj *config_proto.Config,
	notebook_metadata *api_proto.NotebookMetadata) (
	[]*api_proto.NotebookCellRequest, error) {

	// Initialize the notebook from these artifacts
	if len(notebook_metadata.Artifacts) > 0 {
		return getInitialCellsFromArtifacts(ctx, config_obj, notebook_metadata)
	}

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

	return new_cells, nil
}

// Create the initial cells of the notebook.
func (self *NotebookManager) CreateInitialNotebook(ctx context.Context,
	config_obj *config_proto.Config,
	notebook_metadata *api_proto.NotebookMetadata,
	principal string) error {

	new_cell_requests, err := getInitialCells(ctx, config_obj, notebook_metadata)
	if err != nil {
		return err
	}

	// Write the notebook to storage first while we are calculating it.
	err = self.Store.SetNotebook(notebook_metadata)
	if err != nil {
		return err
	}

	for _, cell_req := range new_cell_requests {
		// Skip none cells - they are essentially "commented out"
		if cell_req.Type == "none" {
			continue
		}

		new_cell_id := NewNotebookCellId()

		cell_req.NotebookId = notebook_metadata.NotebookId
		cell_req.CellId = new_cell_id

		// Create the initial version of the cell
		cell_req.Version = GetNextVersion("")
		cell_req.AvailableVersions = append(cell_req.AvailableVersions, cell_req.Version)

		cell_metadata := &api_proto.NotebookCell{
			CellId:            new_cell_id,
			Env:               cell_req.Env,
			Input:             cell_req.Input,
			Output:            cell_req.Output,
			Calculating:       true,
			Type:              cell_req.Type,
			Timestamp:         utils.GetTime().Now().Unix(),
			CurrentVersion:    cell_req.Version,
			AvailableVersions: cell_req.AvailableVersions,
		}

		// Add the new cell to the notebook metadata and fire off the
		// calculation in the background.
		notebook_metadata.CellMetadata = append(
			notebook_metadata.CellMetadata, cell_metadata)
	}

	var final_err error

	// When we create the notebook we need to wait for all the cells
	// to be calculated otherwise we will overwhelm the workers.
	for _, cell_req := range new_cell_requests {
		if cell_req.Type == "none" {
			continue
		}

		cell_req.Sync = true
		_, err = self.UpdateNotebookCell(
			ctx, notebook_metadata, principal, cell_req)
		if err != nil {
			// We failed to create this cell but we should not stop
			// because this will ignore the next cells. Keep going
			// anyway otherwise the next cells will be lost.
			if final_err == nil {
				final_err = err
			}
		}
	}

	return final_err
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

	result := getCustomCells(ctx, config_obj, repository,
		artifact_name, notebook_metadata)

	// If there are no custom cells, add the default cell.
	if len(result) == 0 {
		// Get the start and end times of the timeline displayed in
		// the GUI as a string so we can preserve the GUI timezone.
		start_time, pres := getKeyFromEnv(notebook_metadata.Env, "StartTime")
		if !pres {
			// Start the event display 1 day ago.
			start_time = utils.GetTime().Now().
				AddDate(0, 0, -1).UTC().Format(time.RFC3339)
		}

		end_time, pres := getKeyFromEnv(notebook_metadata.Env, "EndTime")
		if !pres {
			end_time = utils.GetTime().Now().UTC().Format(time.RFC3339)
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

SELECT timestamp(epoch=_ts) AS ServerTime, *
 FROM source(start_time=StartTime, end_time=EndTime)
LIMIT 50
`, artifact_name, start_time, end_time),
		})
	}

	return result
}

func getCustomCells(
	ctx context.Context,
	config_obj *config_proto.Config,
	repository services.Repository,
	source string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {
	var result []*api_proto.NotebookCellRequest

	// Check if the artifact has custom notebook cells defined.
	artifact_source, pres := repository.GetSource(ctx, config_obj, source)
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
			Type:   cell.Type,
			Env:    env,
			Output: cell.Output,
			Sync:   true,
			Input:  cell.Template}

		switch strings.ToLower(cell.Type) {
		case "none":
			result = append(result, request)

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

	dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return nil
	}

	hunt_obj, pres := dispatcher.GetHunt(ctx, hunt_id)
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
LET ERRORS = SELECT ClientId,
       client_info(client_id=ClientId).os_info.hostname AS Hostname,
       FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_uploaded_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'ERROR'

-- Uncomment the below to reissue the exact same hunt to the errored clients
-- SELECT *,
--    hunt_add(client_id=ClientId, hunt_id=HuntId, relaunch=TRUE) AS NewCollection
-- FROM ERRORS

-- Uncomment the below to reissue a new collection and add to the same hunt.
-- You will have to also change "UpdateArtifactName" and add a spec for
-- the parameters. (See docs for collect_client())
-- SELECT *,
--   hunt_add(client_id=ClientId, hunt_id=HuntId,
--     flow_id=collect_client(artifacts="UpdateArtifactName",
--                            client_id=ClientId).flow_id) AS NewCollection
-- FROM ERRORS
SELECT * FROM ERRORS

/*
## Flows with RUNNING status
*/
SELECT ClientId,
       client_info(client_id=ClientId).os_info.hostname AS Hostname,
       FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_uploaded_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'RUNNING'

/*
## Flows with FINISHED status
*/
SELECT ClientId,
       client_info(client_id=ClientId).os_info.hostname AS Hostname,
       FlowId, Flow.start_time As StartedTime,
       Flow.state AS FlowState, Flow.status as FlowStatus,
       Flow.execution_duration as Duration,
       Flow.total_uploaded_bytes as TotalBytes,
       Flow.total_collected_rows as TotalRows
FROM hunt_flows(hunt_id=HuntId)
WHERE FlowState =~ 'Finished'
LIMIT 1000
`})

	return getDefaultCellsForSources(ctx, config_obj, sources, notebook_metadata)
}

func getCellsForFlow(ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id string,
	notebook_metadata *api_proto.NotebookMetadata) []*api_proto.NotebookCellRequest {

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil
	}

	flow_details, err := launcher.GetFlowDetails(
		ctx, config_obj, services.GetFlowOptions{}, client_id, flow_id)
	if err != nil {
		return nil
	}
	flow_context := flow_details.Context

	// Create a cell for each possible source
	var sources []string

	if flow_context.Request != nil {
		manager, err := services.GetRepositoryManager(config_obj)
		if err != nil {
			return nil
		}

		repository, err := manager.GetGlobalRepository(config_obj)
		if err != nil {
			return nil
		}

		for _, artifact_name := range flow_context.Request.Artifacts {
			artifact, pres := repository.Get(ctx, config_obj, artifact_name)
			if !pres {
				continue
			}

			for _, source := range artifact.Sources {
				if source.Name == "" {
					sources = append(sources, artifact.Name)
					break
				}
				sources = append(sources, fmt.Sprintf("%v/%v", artifact.Name, source.Name))
			}
		}
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

	if len(sources) == 0 {
		return []*api_proto.NotebookCellRequest{{
			Type:  "markdown",
			Input: "# Error\n\nNo known artifacts\n",
		}}
	}

	return getDefaultCellsForSources(
		ctx, config_obj, sources, notebook_metadata)
}

func getDefaultCellsForSources(
	ctx context.Context,
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
		artifact, pres := repository.Get(ctx, config_obj, source)
		if pres {
			notebook_metadata.ColumnTypes = append(notebook_metadata.ColumnTypes,
				artifact.ColumnTypes...)
		}

		new_cells := getCustomCells(ctx, config_obj, repository,
			source, notebook_metadata)
		result = append(result, new_cells...)

		// Build a default empty notebook that shows off all the
		// results if there are no custom cells.
		if len(new_cells) == 0 {
			var query string
			orgs, pres := getKeyFromEnv(notebook_metadata.Env, "Orgs")
			if pres && orgs != "" {
				org_ids := []string{}

				for _, o := range strings.Split(orgs, ",") {
					org_ids = append(org_ids, "'''"+o+"'''")
				}
				query = fmt.Sprintf(`
LET Orgs <= (%v)

/*
# %v
*/
SELECT * FROM source(artifact=%q /*, orgs=Orgs */)
LIMIT 50`, strings.Join(org_ids, ", "), source, source)
			} else {
				query = fmt.Sprintf(`
/*
# %v
*/
SELECT * FROM source(artifact=%q)
LIMIT 50
`, source, source)
			}

			result = append(result, &api_proto.NotebookCellRequest{
				Type: "VQL",

				// This env dict overlays on top of the global
				// notebook env where we can find hunt_id, flow_id
				// etc.
				Env: []*api_proto.Env{{
					Key: "ArtifactName", Value: source,
				}},
				Input: query,
			})
		}
	}

	return result
}

func getKeyFromEnv(env []*api_proto.Env, key string) (string, bool) {
	for _, e := range env {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}
