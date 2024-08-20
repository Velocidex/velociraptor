package timelines

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	timelines_proto "www.velocidex.com/golang/velociraptor/timelines/proto"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type AddTimelineFunctionArgs struct {
	Timeline                   string            `vfilter:"required,field=timeline,doc=Supertimeline to add to. If a super timeline does not exist, creates a new one."`
	Name                       string            `vfilter:"required,field=name,doc=Name/Id of child timeline to add."`
	Query                      types.StoredQuery `vfilter:"required,field=query,doc=Run this query to generate the timeline."`
	Key                        string            `vfilter:"required,field=key,doc=The column representing the time to key off."`
	MessageColumn              string            `vfilter:"optional,field=message_column,doc=The column representing the message."`
	TimestampDescriptionColumn string            `vfilter:"optional,field=ts_desc_column,doc=The column representing the timestamp description."`
	NotebookId                 string            `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
}

type AddTimelineFunction struct{}

func (self *AddTimelineFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	arg := &AddTimelineFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("timeline_add: Command can only run on the server")
		return vfilter.Null{}
	}

	notebook_id := arg.NotebookId
	if notebook_id == "" {
		notebook_id = vql_subsystem.GetStringFromRow(scope, scope, "NotebookId")
	}

	if notebook_id == "" {
		scope.Log("timeline_add: Notebook ID must be specified")
		return vfilter.Null{}
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	in := make(chan types.Row)
	go func() {
		defer close(in)

		for event := range arg.Query.Eval(sub_ctx, scope) {
			select {
			case <-ctx.Done():
				break

			case in <- event:
			}
		}

	}()

	super, err := notebook_manager.AddTimeline(sub_ctx, scope,
		notebook_id, arg.Timeline, &timelines_proto.Timeline{
			Id:                         arg.Name,
			TimestampColumn:            arg.Key,
			MessageColumn:              arg.MessageColumn,
			TimestampDescriptionColumn: arg.TimestampDescriptionColumn,
		}, in)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	journal, err := services.GetJournal(config_obj)
	if err == nil {
		journal.PushRowsToArtifactAsync(ctx, config_obj,
			ordereddict.NewDict().
				Set("NotebookId", notebook_id).
				Set("SuperTimelineName", arg.Timeline).
				Set("Action", "AddTimeline").
				Set("Timeline", arg.Name).
				Set("TimestampColumn", arg.Key),
			"Server.Internal.TimelineAdd")
	}

	return super
}

func (self AddTimelineFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "timeline_add",
		Doc:      "Add a new query to a timeline.",
		ArgType:  type_map.AddType(scope, &AddTimelineFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddTimelineFunction{})
}
