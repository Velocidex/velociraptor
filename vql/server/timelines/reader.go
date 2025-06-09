package timelines

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type TimelinePluginArgs struct {
	Timeline          string      `vfilter:"required,field=timeline,doc=Name of the timeline to read"`
	IncludeComponents []string    `vfilter:"optional,field=components,doc=List of child components to include"`
	SkipComponents    []string    `vfilter:"optional,field=skip,doc=List of child components to skip"`
	StartTime         vfilter.Any `vfilter:"optional,field=start,doc=First timestamp to fetch"`
	NotebookId        string      `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
}

type TimelinePlugin struct{}

func (self TimelinePlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "timeline", args)()

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

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("timeline: Command can only run on the server")
			return
		}

		notebook_id := arg.NotebookId
		if notebook_id == "" {
			notebook_id = vql_subsystem.GetStringFromRow(scope, scope, "NotebookId")
		}

		if notebook_id == "" {
			scope.Log("timeline: Notebook ID must be specified")
			return
		}

		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}

		var start time.Time
		if !utils.IsNil(arg.StartTime) {
			start, err = functions.TimeFromAny(ctx, scope, arg.StartTime)
			if err != nil {
				scope.Log("timeline: %v", err)
				return
			}
		}

		events, err := notebook_manager.ReadTimeline(
			ctx, notebook_id, arg.Timeline,
			services.TimelineOptions{
				StartTime:         start,
				IncludeComponents: arg.IncludeComponents,
				ExcludeComponents: arg.SkipComponents})
		if err != nil {
			scope.Log("timeline: %v", err)
			return
		}

		for row := range events.Read(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
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

type TimelineListPluginArgs struct {
	NotebookId string `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
}

type TimelineListPlugin struct{}

func (self TimelineListPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "timelines", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("timelines: %v", err)
			return
		}

		arg := &TimelineListPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("timelines: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("timelines: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("timelines: Command can only run on the server")
			return
		}

		notebook_id := arg.NotebookId
		if notebook_id == "" {
			notebook_id = vql_subsystem.GetStringFromRow(scope, scope, "NotebookId")
		}

		if notebook_id == "" {
			scope.Log("timelines: Notebook ID must be specified")
			return
		}

		notebook_manager, err := services.GetNotebookManager(config_obj)
		if err != nil {
			scope.Log("timelines: %v", err)
			return
		}

		timelines, err := notebook_manager.Timelines(ctx, notebook_id)
		if err != nil {
			scope.Log("timelines: %v", err)
			return
		}

		for _, item := range timelines {
			timelines := []string{}
			for _, t := range item.Timelines {
				timelines = append(timelines, t.Id)
			}
			select {
			case <-ctx.Done():
				return
			case output_chan <- json.ConvertProtoToOrderedDict(item):
			}
		}
	}()

	return output_chan
}

func (self TimelineListPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "timelines",
		Doc:      "List all timelines in a notebook",
		ArgType:  type_map.AddType(scope, &TimelineListPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&TimelinePlugin{})
	vql_subsystem.RegisterPlugin(&TimelineListPlugin{})
}
