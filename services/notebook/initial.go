package notebook

/*
# Creating an initial notebook

There are several types of notebooks:
- Global Notebooks:

  These are initialized from a NOTEBOOK type artifact. The user gets a
  selector to choose which notebook artifact to launch.

  The initial cells are initialized from this artifact - each source
  contains a notebook section with several templates. Cells are
  collected from all sources and added to the final notebook.

- Client Notebooks

  Automatically created in client collections when the user clicks the
  notebook tab. These are normally public.

- Hunt Notebooks

  Automatically created in hunts when the user clicks the notebook tab
  in the hunt viewer. These are normally public.

- Event Notebooks

  Automatically created in event monitoring collections when the user
  clicks the notebook pull down. These are normally public.

To make it simpler to understand the different contexts where
notebooks are created, we always create the initial notebook from a
NOTEBOOK type artifact. When the notebook is created from other
artifacts, the code below creates a psuedo NOTEBOOK artifact based on
the other artifacts and adds it to a private repository.

Notebooks are created by the GUI, when the GUI sends a
NotebookMetadata requests. The following are the important fields:

1. Notebook ID - This can be empty for global notebooks, which will
   generate a new ID. Other notebooks have a well formed standard for
   the ID. For example a Client Notebook contains the flow id and
   client id with the supplied notebook id.

2. artifacts: This is a list of artifact names to start the
   notebook. Each artifact may have a spec but if not, we use the
   default artifact parameters.

3. specs: A list of artifact specs to launch the artifact with.

4. env: An additional list of environment variables to merge with the
   artifact specs.

Once the notebook is created, the code below adds the following fields
to the notebook metadata fields. These fields can be forwarded back by
the GUI in future.

1. parameters: These are the parameters gathered from the custom
   artifact. These may contain additional fields depending on the
   notebook type (For example event notebooks also contain StartTime
   and EndTime, client notebooks contain ClientId etc).

   The GUI may return the parameters to the server, in which case the
   server creates the psuedo notebook artifact from this field.

*/

import (
	"context"
	"fmt"
	"strings"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

// Create the initial cells of the notebook.
func (self *NotebookManager) CreateInitialNotebook(ctx context.Context,
	config_obj *config_proto.Config,
	notebook_metadata *api_proto.NotebookMetadata,
	principal string) error {

	new_cell_requests, notebook_metadata, err := getInitialCells(
		ctx, config_obj, notebook_metadata)
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
			Timestamp:         utils.GetTime().Now().UnixNano(),
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

// Builds the psuedo notebook artifact based on the notebook request.
func CalculateNotebookArtifact(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.NotebookMetadata) (
	res *artifacts_proto.Artifact,
	md *api_proto.NotebookMetadata, ret_err error) {

	out := proto.Clone(in).(*api_proto.NotebookMetadata)

	// No notebook Id will allocate a global ID.
	if out.NotebookId == "" {
		out.NotebookId = NewNotebookId()
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return nil, nil, err
	}

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, nil, err
	}

	// If no artifacts are specified, we use the default template.
	if len(out.Artifacts) == 0 {
		err := populateDefaultSpecs(ctx, config_obj, out)
		if err != nil {
			return nil, nil, err
		}
	}

	if len(out.Artifacts) == 0 {
		out.Artifacts = append(out.Artifacts, "Notebooks.Default")
	}

	// This is a psuedo artifact used to build the notebook.
	res = &artifacts_proto.Artifact{
		Name: "PrivateNotebook",
	}

	// Check if the psuedo artifact is already cached.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, nil, err
	}

	notebook_path_manager := paths.NewNotebookPathManager(out.NotebookId)
	err = db.GetSubject(config_obj,
		notebook_path_manager.Artifact(),
		res)
	if err == nil {
		// Artifact is cached, lets return that
		out.Parameters = res.Parameters
		return res, out, nil
	}

	// Cache it for next time.
	defer func() {
		err1 := db.SetSubject(
			config_obj, notebook_path_manager.Artifact(), res)
		if err1 != nil && ret_err == nil {
			ret_err = err1
		}
	}()

	// Now build the psuedo artifact.
	seen := make(map[string]bool)
	seen_tools := make(map[string]bool)

	for idx, artifact_name := range out.Artifacts {
		artifact, pres := global_repository.Get(ctx, config_obj, artifact_name)
		if !pres {
			return nil, nil, fmt.Errorf("Artifact not found: %v: %w",
				artifact_name, utils.NotFoundError)
		}

		// Copy out all the exports from the artifact
		res.Export += artifact.Export

		// Resolve any imports - expand their export directly into the
		// psuedo artifact's export section because it will be
		// compiled in a private repository, so wont be able to see
		// the imported artifact definitions.
		for _, imp := range artifact.Imports {
			dep, pres := global_repository.Get(ctx, config_obj, imp)
			if !pres {
				return nil, nil, fmt.Errorf(
					"Artifact %v imports artifact %v which is not known",
					artifact_name, imp)
			}

			res.Export += dep.Export
		}

		// Copy out all the tools
		for _, t := range artifact.Tools {
			_, pres := seen_tools[t.Name]
			if pres {
				continue
			}
			seen_tools[t.Name] = true
			res.Tools = append(res.Tools, t)
		}

		// Copy out column types
		for _, ct := range artifact.ColumnTypes {
			res.ColumnTypes = append(res.ColumnTypes, ct)
		}

		// Copy out all the parameters
		for _, p := range artifact.Parameters {
			_, pres := seen[p.Name]
			if pres {
				continue
			}
			seen[p.Name] = true
			res.Parameters = append(res.Parameters, p)
		}

		// If there are no sources in this artifact add a single fake
		// source so we can do something.
		sources := artifact.Sources
		if len(sources) == 0 {
			sources = append(sources, &artifacts_proto.ArtifactSource{})
		}

		for _, s := range sources {
			new_source := &artifacts_proto.ArtifactSource{
				Name: s.Name,
			}
			res.Sources = append(res.Sources, new_source)

			source_name := artifact_name
			if new_source.Name != "" {
				source_name += "/" + new_source.Name
			}

			// If there are too many cells we add a placeholder to
			// allow the user to calculate them on demand. Otherwise
			// we may overwhelm the notebook workers.
			output := ""
			if idx > 4 {
				output = fmt.Sprintf("<h3>%s</h3><br>Recalculate to View", source_name)
			}

			custom_cells := false
			for _, n := range s.Notebook {
				new_source.Notebook = append(new_source.Notebook, n)
				switch strings.ToLower(n.Type) {

				// Artifacts may set a notebook cell to type "none" to
				// declare a custom notebook which will not actually
				// be used. This allows suppressing notebook cells for
				// this source.
				case "vql", "md", "markdown", "none":
					custom_cells = true
				}
			}

			if !custom_cells {
				// No notebook specified for this source, add a
				// default.

				default_limit := int64(50)
				if config_obj.Defaults != nil &&
					config_obj.Defaults.NotebookDefaultNewCellRows > 0 {
					default_limit = config_obj.Defaults.NotebookDefaultNewCellRows
				}

				switch paths.ModeNameToMode(artifact.Type) {
				case paths.MODE_CLIENT_EVENT, paths.MODE_SERVER_EVENT:
					new_source.Notebook = append(new_source.Notebook,
						&artifacts_proto.NotebookSourceCell{
							Type:   "vql",
							Output: output,
							Template: fmt.Sprintf(`
/*
# Events from %v

From {{ Scope "StartTime" }} to {{ Scope "EndTime" }}
*/

SELECT timestamp(epoch=_ts) AS ServerTime, *
 FROM source(start_time=StartTime, end_time=EndTime, artifact=%q)
LIMIT %v
`, source_name, source_name, default_limit),
						})

				default:
					new_source.Notebook = append(new_source.Notebook,
						&artifacts_proto.NotebookSourceCell{
							Type:   "vql",
							Output: output,
							Template: fmt.Sprintf(`
/*
# %v
*/
SELECT * FROM source(artifact=%q)
LIMIT 50
`, source_name, source_name),
						})
				}
			}
		}
	}

	if len(out.Artifacts) > 0 {
		res.Parameters = append(res.Parameters,
			&artifacts_proto.ArtifactParameter{
				Name:        "ArtifactName",
				Description: "Name of the artifact this notebook came from.",
				Default:     out.Artifacts[0],
			})
	}

	// Add any custom variables.
	flow_id, client_id, ok := utils.ClientNotebookId(out.NotebookId)
	if ok {
		res.Parameters = append(res.Parameters,
			[]*artifacts_proto.ArtifactParameter{{
				Name:        "ClientId",
				Description: "Implied client id from notebook",
				Default:     client_id,
			}, {
				Name:        "FlowId",
				Description: "Implied flow id from notebook",
				Default:     flow_id,
			}}...)

		res.Sources = append(res.Sources, &artifacts_proto.ArtifactSource{
			Notebook: []*artifacts_proto.NotebookSourceCell{{
				Name: "Collection logs",
				Type: "vql_suggestion",
				Template: `
/*
# Flow logs
*/

SELECT * FROM flow_logs(client_id=ClientId, flow_id=FlowId)
`,
			}}})
	}

	_, client_id, ok = utils.EventNotebookId(out.NotebookId)
	if ok {
		res.Parameters = append(res.Parameters,
			[]*artifacts_proto.ArtifactParameter{
				{
					Name:        "ClientId",
					Description: "Implied client id from notebook",
					Default:     client_id,
				},
				{
					Name:        "StartTime",
					Description: "Start of time range to consider",
					Type:        "timestamp",
				},
				{
					Name:        "EndTime",
					Description: "End of time range to consider",
					Type:        "timestamp",
				},
			}...)
	}

	hunt_id, ok := utils.HuntNotebookId(out.NotebookId)
	if ok {
		res.Parameters = append(res.Parameters,
			&artifacts_proto.ArtifactParameter{
				Name:        "HuntId",
				Description: "Implied hunt id from notebook",
				Default:     hunt_id,
			})

		// Add some hunt specific suggestions
		res.Sources = append(res.Sources, &artifacts_proto.ArtifactSource{
			Notebook: []*artifacts_proto.NotebookSourceCell{{
				Name: "Hunt Progress",
				Type: "vql_suggestion",
				Template: `

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
`,
			}}})
	}

	// Keep the psuedo artifact's parameters list in the notebook metadata.
	out.Parameters = res.Parameters

	return res, out, nil
}

// Populates the specs from defaults:
//  1. If this is a client artifact, specs are populated from the flow
//     request.
//  2. For hunt artifacts the specs are populated from hunt object.
//  3. For event artifacts the specs are populated from the client or
//     server monitoring tables.
func populateDefaultSpecs(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.NotebookMetadata) error {
	// Is it a client notebook?
	flow_id, client_id, ok := utils.ClientNotebookId(in.NotebookId)
	if ok {
		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			return err
		}

		flow_obj, err := launcher.GetFlowDetails(ctx, config_obj,
			services.GetFlowOptions{}, client_id, flow_id)
		if err != nil {
			return err
		}

		if flow_obj.Context != nil &&
			flow_obj.Context.Request != nil {
			req := flow_obj.Context.Request
			in.Artifacts = req.Artifacts
			in.Specs = req.Specs
		}
		return nil
	}

	hunt_id, ok := utils.HuntNotebookId(in.NotebookId)
	if ok {
		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			return err
		}

		hunt_obj, ok := hunt_dispatcher.GetHunt(ctx, hunt_id)
		if !ok {
			return fmt.Errorf("Hunt not found: %v: %w",
				hunt_id, utils.NotFoundError)
		}

		if hunt_obj.StartRequest != nil {
			req := hunt_obj.StartRequest
			in.Artifacts = req.Artifacts
			in.Specs = req.Specs
		}
		return nil
	}

	artifact_name, client_id, ok := utils.EventNotebookId(in.NotebookId)
	if ok {
		specs, err := getSpecFromEventArtifact(ctx, config_obj,
			artifact_name, client_id)
		if err != nil {
			return err
		}

		in.Artifacts = []string{artifact_name}
		in.Specs = specs
	}

	return nil
}

// Given the psuedo notebook artifact and the pre-populated request,
// calculate the specs required to launch the notebook artifact.
func CalculateSpecs(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	in *api_proto.NotebookMetadata) (*flows_proto.ArtifactSpec, error) {

	// Populate specs if they are not specified.
	if in.Specs == nil {
		err := populateDefaultSpecs(ctx, config_obj, in)
		if err != nil {
			return nil, err
		}
	}

	// The caller can set a Specs set OR set seperate env
	seen := make(map[string]string)
	for _, s := range in.Specs {
		if s.Parameters != nil {
			for _, e := range s.Parameters.Env {
				seen[e.Key] = e.Value
			}
		}
	}

	for _, e := range in.Env {
		seen[e.Key] = e.Value
	}

	res := &flows_proto.ArtifactSpec{
		Artifact:   artifact.Name,
		Parameters: &flows_proto.ArtifactParameters{},
	}

	for _, p := range artifact.Parameters {
		v, pres := seen[p.Name]
		if pres {
			res.Parameters.Env = append(res.Parameters.Env,
				&actions_proto.VQLEnv{
					Key:   p.Name,
					Value: v,
				})
		}
	}

	return res, nil
}

// Compile the psuedo artifact into a set of requests that can be used
// to recreate VQL state. These requests are added to the notebook
// metadata.
func updateNotebookRequests(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	spec *flows_proto.ArtifactSpec,
	in *api_proto.NotebookMetadata) error {

	// Create a child repository as we will need to update the
	// artifact definitions.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return err
	}

	global_repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// The new repository is isolated but will search the global
	// repository for any artifacts it does not know about.
	repository := manager.NewRepository()
	repository.SetParent(global_repository, config_obj)

	_, err = repository.LoadProto(artifact, services.ArtifactOptions{})
	if err != nil {
		return err
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	acl_manager := acl_managers.NullACLManager{}

	in.Requests, err = launcher.CompileCollectorArgs(
		ctx, config_obj, acl_manager, repository,
		services.CompilerOptions{
			DisablePrecondition: true,
		},
		&flows_proto.ArtifactCollectorArgs{
			Artifacts: []string{artifact.Name},
			Specs:     []*flows_proto.ArtifactSpec{spec},
		})
	if err != nil {
		return err
	}

	ct_var := ordereddict.NewDict()
	for _, ct := range artifact.ColumnTypes {
		ct_var.Set(ct.Name, ct.Type)
	}

	for _, r := range in.Requests {
		r.Env = append(r.Env, &actions_proto.VQLEnv{
			Key:   "ColumnTypes",
			Value: json.MustMarshalString(ct_var),
		})

		r.Query = append([]*actions_proto.VQLRequest{
			{VQL: "LET ColumnTypes <= parse_json(data=ColumnTypes)"},
		}, r.Query...)
	}

	return nil
}

// Get the initial cells from a notebook artifact. Each source should
// contain a notebook clause.
func getInitialCellsFromArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact,
	in *api_proto.NotebookMetadata) (
	result []*api_proto.NotebookCellRequest, err error) {

	for _, s := range artifact.Sources {
		for _, n := range s.Notebook {
			var env []*api_proto.Env

			// Allow the notebook to specify env variables per
			// source.
			for _, i := range n.Env {
				env = append(env, &api_proto.Env{
					Key:   i.Key,
					Value: i.Value,
				})
			}

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
	return result, nil
}

func getInitialCells(
	ctx context.Context,
	config_obj *config_proto.Config,
	in *api_proto.NotebookMetadata) (
	[]*api_proto.NotebookCellRequest, *api_proto.NotebookMetadata, error) {

	psuedo_artifact, out, err := CalculateNotebookArtifact(
		ctx, config_obj, in)
	if err != nil {
		return nil, nil, err
	}

	spec, err := CalculateSpecs(ctx, config_obj, psuedo_artifact, out)
	if err != nil {
		return nil, nil, err
	}

	// Add the VQL requests to the notebook
	err = updateNotebookRequests(ctx, config_obj, psuedo_artifact, spec, out)
	if err != nil {
		return nil, nil, err
	}

	cells, err := getInitialCellsFromArtifacts(ctx, config_obj, psuedo_artifact, out)
	if err != nil {
		return nil, nil, err
	}

	return cells, out, err
}

// Analyze the event table to extract the parameters that the event
// artifact was launched with.
func getSpecFromEventArtifact(
	ctx context.Context,
	config_obj *config_proto.Config,
	artifact, client_id string) (res []*flows_proto.ArtifactSpec, err error) {

	if client_id == "server" {
		server_monitoring_service, err := services.GetServerEventManager(
			config_obj)
		if err != nil {
			return nil, err
		}

		for _, spec := range server_monitoring_service.Get().Specs {
			if spec.Artifact == artifact {
				res = append(res, spec)
				return res, nil
			}
		}
	} else {
		client_event_manager, err := services.ClientEventManager(config_obj)
		if err != nil {
			return nil, err
		}

		for _, spec := range client_event_manager.GetClientSpec(
			ctx, config_obj, client_id) {
			if spec.Artifact == artifact {
				res = append(res, spec)
				return res, nil
			}
		}
	}

	res = append(res, &flows_proto.ArtifactSpec{
		Artifact: artifact,
	})
	return res, nil
}
