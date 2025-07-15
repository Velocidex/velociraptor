package flows

import (
	"context"
	"errors"
	"runtime"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// This is very similar to the source plugin, but runs the query over
// subsets of the sources in parallel, combining results.
type ParallelPluginArgs struct {
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
	NotebookId        string `vfilter:"optional,field=notebook_id,doc=The notebook to read from (should also include cell id)"`
	NotebookCellId    string `vfilter:"optional,field=notebook_cell_id,doc=The notebook cell read from (should also include notebook id)"`
	NotebookCellTable int64  `vfilter:"optional,field=notebook_cell_table,doc=A notebook cell can have multiple tables.)"`

	Query vfilter.StoredQuery `vfilter:"required,field=query,doc=The query will be run in parallel over batches."`

	Workers   int64 `vfilter:"optional,field=workers,doc=Number of workers to spawn.)"`
	BatchSize int64 `vfilter:"optional,field=batch,doc=Number of rows in each batch.)"`

	source_arg *SourcePluginArgs
}

func (self *ParallelPluginArgs) DetermineMode(
	ctx context.Context, config_obj *config_proto.Config,
	scope vfilter.Scope, args *ordereddict.Dict) error {
	self.source_arg = &SourcePluginArgs{
		ClientId:          self.ClientId,
		FlowId:            self.FlowId,
		HuntId:            self.HuntId,
		Artifact:          self.Artifact,
		Source:            self.Source,
		StartTime:         self.StartTime,
		EndTime:           self.EndTime,
		NotebookId:        self.NotebookId,
		NotebookCellId:    self.NotebookCellId,
		NotebookCellTable: self.NotebookCellTable,
	}
	return self.source_arg.DetermineMode(ctx, config_obj, scope, args)
}

type ParallelPlugin struct{}

func (self ParallelPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "parallel", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("parallel: %s", err)
			return
		}

		arg := &ParallelPluginArgs{}
		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("parallel: Command can only run on the server")
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("parallel: %v", err)
			return
		}

		// Determine the mode based on the args passed.
		err = arg.DetermineMode(ctx, config_obj, scope, args)
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

		job_chan, err := breakIntoScopes(ctx, config_obj, scope, arg)
		if err != nil {
			scope.Log("parallel: %v", err)
			return
		}

		for i := int64(0); i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				for job := range job_chan {
					client_id, _ := job.GetString("ClientId")
					flow_id, _ := job.GetString("FlowId")

					subscope := scope.Copy()
					subscope.AppendVars(job)

					for row := range arg.Query.Eval(ctx, subscope) {
						// When operating on a hunt we tag each row
						// with its client id and flow id
						if arg.HuntId != "" {
							row_dict, ok := row.(*ordereddict.Dict)
							if ok {
								row_dict.Set("ClientId", client_id).
									Set("FlowId", flow_id)
							}
						}

						select {
						case <-ctx.Done():
							return
						case output_chan <- row:
						}
					}
				}
			}()
		}

		wg.Wait()
	}()

	return output_chan
}

func (self ParallelPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "parallelize",
		Doc:      "Runs query on result batches in parallel.",
		ArgType:  type_map.AddType(scope, &ParallelPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

// A testable utility function that breaks the request into a set of
// args to the source() plugins.
func breakIntoScopes(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	arg *ParallelPluginArgs) (<-chan *ordereddict.Dict, error) {

	// Handle hunts especially.
	if arg.source_arg.mode == MODE_HUNT_ARTIFACT {
		return breakHuntIntoScopes(ctx, config_obj, scope, arg)
	}

	// Other sources are strored in a single reader.  Depending on
	// the parameters, we need to get the reader from different
	// places.
	var err error
	var result_set_reader result_sets.ResultSetReader
	output_chan := make(chan *ordereddict.Dict)

	if arg.source_arg.mode == MODE_NOTEBOOK {
		result_set_reader, err = getNotebookResultSetReader(
			ctx, config_obj, scope, arg.source_arg)

	} else if arg.source_arg.mode == MODE_FLOW_ARTIFACT {
		result_set_reader, err = getFlowResultSetReader(
			ctx, config_obj, scope, arg.source_arg)

	} else {
		err = errors.New("Unknown mode")
	}

	if err != nil {
		close(output_chan)
		return output_chan, err
	}

	// Figure how large the result set is.
	total_rows := result_set_reader.TotalRows()
	result_set_reader.Close()

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
			select {
			case <-ctx.Done():
				return

			case output_chan <- ordereddict.NewDict().
				Set("ClientId", arg.source_arg.ClientId).
				Set("FlowId", arg.source_arg.FlowId).

				// Mask hunt id since we already take
				// care of it in breakHuntIntoScopes
				// and we dont want source() plugin to
				// pick it up.
				Set("HuntId", "").
				Set("ArtifactName", arg.source_arg.Artifact).
				Set("StartTime", arg.source_arg.StartTime).
				Set("EndTime", arg.source_arg.EndTime).
				Set("NotebookId", arg.source_arg.NotebookId).
				Set("NotebookCellId", arg.source_arg.NotebookCellId).
				Set("NotebookCellTable", arg.source_arg.NotebookCellTable).
				Set("StartRow", i).
				Set("Limit", step_size):
			}
		}

	}()

	return output_chan, nil
}

func breakHuntIntoScopes(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	arg *ParallelPluginArgs) (<-chan *ordereddict.Dict, error) {

	output_chan := make(chan *ordereddict.Dict)
	go func() {
		defer close(output_chan)

		hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			return
		}

		options := services.FlowSearchOptions{BasicInformation: true}
		flow_chan, _, err := hunt_dispatcher.GetFlows(
			ctx, config_obj, options, scope, arg.source_arg.HuntId, 0)
		if err != nil {
			return
		}

		for flow_details := range flow_chan {
			if flow_details == nil || flow_details.Context == nil {
				continue
			}

			client_id := flow_details.Context.ClientId
			flow_id := flow_details.Context.SessionId
			arg := &ParallelPluginArgs{
				Artifact:  arg.source_arg.Artifact,
				ClientId:  client_id,
				FlowId:    flow_id,
				Workers:   arg.Workers,
				BatchSize: arg.BatchSize,
			}

			err = arg.DetermineMode(ctx, config_obj, scope, nil)
			if err != nil {
				continue
			}

			flow_job, err := breakIntoScopes(ctx, config_obj, scope, arg)
			if err == nil {
				for job := range flow_job {
					select {
					case <-ctx.Done():
						return
					case output_chan <- job:
					}
				}
			}
		}

	}()

	return output_chan, nil
}

func init() {
	vql_subsystem.RegisterPlugin(&ParallelPlugin{})
}
