// +build server_vql

package server

import (
	"context"
	"runtime"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/result_sets"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// This is very similar to the source plugin, but runs the query over
// subsets of the sources in parallel, combining results.
type ParallelPluginArgs struct {
	Query vfilter.StoredQuery `vfilter:"required,field=query,doc=The query will be run in parallel over batches."`

	// Collected artifacts from clients should specify the client
	// id and flow id as well as the artifact and source.
	ClientId string `vfilter:"optional,field=client_id,doc=The client id to extract"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=A flow ID (client or server artifacts)"`

	// Specifying the hunt id will retrieve all rows in this hunt
	// (from all clients). You still need to specify the artifact
	// name.
	HuntId string `vfilter:"optional,field=hunt_id,doc=Retrieve sources from this hunt (combines all results from all clients)"`

	// Artifacts are specified by name and source. Name may
	// include the source following the artifact name by a slash -
	// e.g. Custom.Artifact/SourceName.
	Artifact string `vfilter:"optional,field=artifact,doc=The name of the artifact collection to fetch"`
	Source   string `vfilter:"optional,field=source,doc=An optional named source within the artifact"`

	// If the artifact name specifies an event artifact, you may
	// also specify start and end times to return.
	StartTime int64 `vfilter:"optional,field=start_time,doc=Start return events from this date (for event sources)"`
	EndTime   int64 `vfilter:"optional,field=end_time,doc=Stop end events reach this time (event sources)."`

	// A source may specify a notebook cell to read from - this
	// allows post processing in multiple stages - one query
	// reduces the data into a result set and subsequent queries
	// operate on that reduced set.
	NotebookId        string `vfilter:"optional,field=notebook_id,doc=The notebook to read from (shoud also include cell id)"`
	NotebookCellId    string `vfilter:"optional,field=notebook_cell_id,doc=The notebook cell read from (shoud also include notebook id)"`
	NotebookCellTable int64  `vfilter:"optional,field=notebook_cell_table,doc=A notebook cell can have multiple tables.)"`

	Workers   int64 `vfilter:"optional,field=workers,doc=Number of workers to spawn.)"`
	BatchSize int64 `vfilter:"optional,field=batch,doc=Number of rows in each batch.)"`
}

type ParallelPlugin struct{}

func (self ParallelPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("parallel: %s", err)
			return
		}

		arg := &ParallelPluginArgs{}
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		err = vfilter.ExtractArgs(scope, args, arg)
		if err != nil {
			scope.Log("parallel: %v", err)
			return
		}

		wg := sync.WaitGroup{}
		workers := arg.Workers
		if workers == 0 {
			// By default use all the cpus.
			workers = int64(runtime.NumCPU())
		}

		job_chan, err := breakIntoScopes(ctx, config_obj, arg)
		if err != nil {
			scope.Log("parallel: %v", err)
			return
		}

		for i := int64(0); i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for job := range job_chan {
					subscope := scope.Copy()
					subscope.AppendVars(job)

					for row := range arg.Query.Eval(ctx, subscope) {
						output_chan <- row
					}
				}
			}()
		}

		wg.Wait()
	}()

	return output_chan
}

func (self ParallelPlugin) Info(
	scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parallelize",
		Doc:     "Runs query on result batches in parallel.",
		ArgType: type_map.AddType(scope, &ParallelPluginArgs{}),
	}
}

// A testable utility function that breaks the request into a set of
// args to the source() plugins.
func breakIntoScopes(
	ctx context.Context,
	config_obj *config_proto.Config,
	arg *ParallelPluginArgs) (<-chan *ordereddict.Dict, error) {

	// Handle hunts especially.
	if arg.HuntId != "" {
		return breakHuntIntoScopes(ctx, config_obj, arg)
	}

	// Other sources are strored in a single reader.  Depending on
	// the parameters, we need to get the reader from different
	// places.
	result_set_reader, err := getResultSetReader(
		ctx, config_obj, &SourcePluginArgs{
			ClientId:          arg.ClientId,
			FlowId:            arg.FlowId,
			Artifact:          arg.Artifact,
			Source:            arg.Source,
			StartTime:         arg.StartTime,
			EndTime:           arg.EndTime,
			NotebookId:        arg.NotebookId,
			NotebookCellId:    arg.NotebookCellId,
			NotebookCellTable: arg.NotebookCellTable,
		})
	if err != nil {
		return nil, err
	}

	// Figure how large the result set is.
	total_rows := result_set_reader.TotalRows()
	result_set_reader.Close()

	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		step_size := arg.BatchSize
		if step_size == 0 {
			step_size = total_rows / 10
			if step_size < 1000 {
				step_size = 1000
			}
		}

		for i := int64(0); i < total_rows; i += step_size {
			output_chan <- ordereddict.NewDict().
				Set("ClientId", arg.ClientId).
				Set("FlowId", arg.FlowId).

				// Mask hunt id since we already take
				// care of it in breakHuntIntoScopes
				// and we dont want source() plugin to
				// pick it up.
				Set("HuntId", "").
				Set("ArtifactName", arg.Artifact).
				Set("StartTime", arg.StartTime).
				Set("EndTime", arg.EndTime).
				Set("NotebookId", arg.NotebookId).
				Set("NotebookCellId", arg.NotebookCellId).
				Set("NotebookCellTable", arg.NotebookCellTable).
				Set("StartRow", i).
				Set("Limit", step_size)
		}

	}()

	return output_chan, nil
}

func breakHuntIntoScopes(
	ctx context.Context,
	config_obj *config_proto.Config,
	arg *ParallelPluginArgs) (<-chan *ordereddict.Dict, error) {
	file_store_factory := file_store.GetFileStore(config_obj)

	hunt_path_manager := paths.NewHuntPathManager(arg.HuntId).Clients()
	hunt_rs_reader, err := result_sets.NewResultSetReader(
		file_store_factory, hunt_path_manager)
	if err != nil {
		return nil, err
	}

	output_chan := make(chan *ordereddict.Dict)
	go func() {
		defer close(output_chan)

		for row := range hunt_rs_reader.Rows(ctx) {
			client_id, _ := row.GetString("ClientId")
			flow_id, _ := row.GetString("FlowId")

			flow_job, err := breakIntoScopes(ctx, config_obj,
				&ParallelPluginArgs{
					Artifact:  arg.Artifact,
					ClientId:  client_id,
					FlowId:    flow_id,
					Workers:   arg.Workers,
					BatchSize: arg.BatchSize,
				})
			if err == nil {
				for job := range flow_job {
					output_chan <- job
				}
			}
		}

	}()

	return output_chan, nil
}

func init() {
	vql_subsystem.RegisterPlugin(&ParallelPlugin{})
}
