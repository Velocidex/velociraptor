package timelines

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/timelines"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sorter"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type AddTimelineFunctionArgs struct {
	Timeline   string            `vfilter:"required,field=timeline,doc=Supertimeline to add to"`
	Name       string            `vfilter:"required,field=name,doc=Name of child timeline"`
	Query      types.StoredQuery `vfilter:"required,field=query,doc=Run this query to generate the timeline."`
	Key        string            `vfilter:"required,field=key,doc=The column representing the time."`
	NotebookId string            `vfilter:"optional,field=notebook_id,doc=The notebook ID the timeline is stored in."`
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

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
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

	notebook_path_manager := paths.NewNotebookPathManager(notebook_id)
	super, err := timelines.NewSuperTimelineWriter(
		config_obj, notebook_path_manager.Timeline(arg.Timeline))
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}
	defer super.Close()

	// make a new timeline to store in the super timeline.
	writer, err := super.AddChild(arg.Name)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}
	defer writer.Close()

	writer.Truncate()

	subscope := scope.Copy()
	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Timelines have to be sorted, so we force them to be sorted
	// by the key.
	sorter := sorter.MergeSorter{10000}
	sorted_chan := sorter.Sort(sub_ctx, subscope, arg.Query.Eval(sub_ctx, subscope),
		arg.Key, false /* desc */)

	for row := range sorted_chan {
		key, pres := scope.Associative(row, arg.Key)
		if !pres {
			scope.Log("timeline_add: Key %v is not found in query", arg.Key)
			return vfilter.Null{}
		}

		ts, err := functions.TimeFromAny(scope, key)
		if err == nil {
			writer.Write(ts, vfilter.RowToDict(sub_ctx, subscope, row))
		}
	}

	// Now record the new timeline in the notebook if needed.
	db, _ := datastore.GetDB(config_obj)
	notebook_metadata := &api_proto.NotebookMetadata{}
	err = db.GetSubject(config_obj, notebook_path_manager.Path(), notebook_metadata)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	for _, item := range notebook_metadata.Timelines {
		if item == arg.Timeline {
			return super.SuperTimeline
		}
	}

	notebook_metadata.Timelines = append(notebook_metadata.Timelines, arg.Timeline)
	err = db.SetSubject(config_obj, notebook_path_manager.Path(), notebook_metadata)
	if err != nil {
		scope.Log("timeline_add: %v", err)
		return vfilter.Null{}
	}

	return super.SuperTimeline
}

func (self AddTimelineFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "timeline_add",
		Doc:     "Add a new query to a timeline.",
		ArgType: type_map.AddType(scope, &AddTimelineFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddTimelineFunction{})
}
