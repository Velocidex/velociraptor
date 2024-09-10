package timelines

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteTimelineFunctionArgs struct {
	Timeline   string `vfilter:"required,field=timeline,doc=Supertimeline to delete."`
	NotebookId string `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
	Component  string `vfilter:"optional,field=name,doc=Name/Id of child timeline to delete. If not specified deletes the entire timeline"`
}

type DeleteTimelineFunction struct{}

func (self *DeleteTimelineFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.NOTEBOOK_EDITOR)
	if err != nil {
		scope.Log("timeline_delete: %v", err)
		return vfilter.Null{}
	}

	arg := &DeleteTimelineFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("timeline_delete: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("timeline_delete: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("timeline_delete: Command can only run on the server")
		return vfilter.Null{}
	}

	notebook_id := arg.NotebookId
	if notebook_id == "" {
		notebook_id = vql_subsystem.GetStringFromRow(scope, scope, "NotebookId")
	}

	if notebook_id == "" {
		scope.Log("timeline_delete: Notebook ID must be specified")
		return vfilter.Null{}
	}

	notebook_manager, err := services.GetNotebookManager(config_obj)
	if err != nil {
		scope.Log("timeline_delete: %v", err)
		return vfilter.Null{}
	}

	err = notebook_manager.DeleteTimeline(
		ctx, scope, notebook_id, arg.Timeline, arg.Component)
	if err != nil {
		scope.Log("timeline_delete: %v", err)
		return vfilter.Null{}
	}

	journal, err := services.GetJournal(config_obj)
	if err == nil {
		journal.PushRowsToArtifactAsync(ctx, config_obj,
			ordereddict.NewDict().
				Set("NotebookId", notebook_id).
				Set("SuperTimelineName", arg.Timeline).
				Set("Component", arg.Component).
				Set("Action", "Delete"),
			"Server.Internal.TimelineAdd")
	}

	return true
}

func (self DeleteTimelineFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "timeline_delete",
		Doc:      "Delete a super timeline.",
		ArgType:  type_map.AddType(scope, &DeleteTimelineFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.NOTEBOOK_EDITOR).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&DeleteTimelineFunction{})
}
