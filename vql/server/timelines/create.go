package timelines

import (
	"context"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/timelines"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type AddTimelineFunctionArgs struct {
	Timeline string            `vfilter:"required,field=timeline,doc=Supertimeline to add to"`
	Name     string            `vfilter:"required,field=name,doc=Name of child timeline"`
	Query    types.StoredQuery `vfilter:"required,field=query,doc=Run this query to generate the timeline."`
	Key      string            `vfilter:"required,field=key,doc=The column representing the time."`
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

	super, err := timelines.NewSuperTimelineWriter(config_obj, arg.Timeline)
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

	for row := range arg.Query.Eval(sub_ctx, subscope) {
		key, pres := scope.Associative(row, arg.Key)
		if !pres {
			scope.Log("timeline_add: Key %v is not found in query", arg.Key)
			return vfilter.Null{}
		}

		ts, ok := key.(time.Time)
		if !ok {
			scope.Log("timeline_add: Key %v is not a timestamp it is of type %T", arg.Key, key)
			return vfilter.Null{}
		}

		writer.Write(ts, vfilter.RowToDict(sub_ctx, subscope, row))
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
