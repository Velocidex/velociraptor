package functions

import (
	"context"
	"math/rand"
	"time"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type SleepArgs struct {
	Sleep int64 `vfilter:"optional,field=time,doc=The number of seconds to sleep"`
}

type SleepFunction struct{}

func (self *SleepFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &SleepArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("sleep: %s", err.Error())
		return false
	}

	select {
	// Cancellation should abort the sleep.
	case <-ctx.Done():
		break

	case <-time.After(time.Duration(arg.Sleep) * time.Second):
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
	err := vfilter.ExtractArgs(scope, args, arg)
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
