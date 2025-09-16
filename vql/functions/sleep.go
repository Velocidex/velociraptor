package functions

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/utils/rand"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SleepArgs struct {
	Sleep int64 `vfilter:"optional,field=time,doc=The number of seconds to sleep"`
	MS    int64 `vfilter:"optional,field=ms,doc=The number of ms to sleep"`
}

type SleepFunction struct{}

func (self *SleepFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "sleep", args)()

	arg := &SleepArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("sleep: %s", err.Error())
		return false
	}

	ms := arg.MS
	if ms == 0 {
		ms = arg.Sleep * 1000
	}

	select {
	// Cancellation should abort the sleep.
	case <-ctx.Done():
		break

	case <-time.After(time.Duration(ms) * time.Millisecond):
		break
	}

	return true
}

func (self *SleepFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "sleep",
		Doc:     "Sleep for the specified number of seconds. Always returns true.",
		ArgType: type_map.AddType(scope, &SleepArgs{}),
	}
}

type RandArgs struct {
	Range int64 `vfilter:"optional,field=range,doc=Selects a random number up to this range."`
}

type RandFunction struct{}

func (self *RandFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &RandArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rand: %s", err.Error())
		return false
	}

	if arg.Range == 0 {
		return 0
	}

	return rand.Intn(int(arg.Range))
}

func (self RandFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rand",
		Doc:     "Selects a random number.",
		ArgType: type_map.AddType(scope, &RandArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SleepFunction{})
	vql_subsystem.RegisterFunction(&RandFunction{})
}
