package timelines

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/timelines"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type TimelinePluginArgs struct {
	Timeline       string      `vfilter:"required,field=timeline,doc=Name of the timeline to read"`
	SkipComponents []string    `vfilter:"optional,field=skip,doc=List of child components to skip"`
	StartTime      vfilter.Any `vfilter:"optional,field=start,doc=First timestamp to fetch"`
	NotebookId     string      `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
}

type TimelinePlugin struct{}

func (self TimelinePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}

		arg := &TimelinePluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		notebook_id := arg.NotebookId
		if notebook_id == "" {
			notebook_id = vql_subsystem.GetStringFromRow(scope, scope, "NotebookId")
		}

		if notebook_id == "" {
			scope.Log("timeline_add: Notebook ID must be specified")
			return
		}

		super_path_manager := paths.NewNotebookPathManager(notebook_id).
			SuperTimeline(arg.Timeline)

		reader, err := timelines.NewSuperTimelineReader(config_obj,
			super_path_manager, arg.SkipComponents)
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}
		defer reader.Close()

		if !utils.IsNil(arg.StartTime) {
			start, err := functions.TimeFromAny(scope, arg.StartTime)
			if err != nil {
				scope.Log("timeline: %v", err)
				return
			}

			reader.SeekToTime(start)
		}

		for item := range reader.Read(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- item.Row.Set("_ts", item.Time):
			}
		}
	}()

	return output_chan
}

func (self TimelinePlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "timeline",
		Doc:      "Read a timeline. You can create a timeline with the timeline_add() function",
		ArgType:  type_map.AddType(scope, &TimelinePluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&TimelinePlugin{})
}
