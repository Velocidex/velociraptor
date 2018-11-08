package functions

import (
	"context"
	"fmt"

	humanize "github.com/dustin/go-humanize"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type HumanizeArgs struct {
	Bytes int64 `vfilter:"optional,field=bytes"`
}

type HumanizeFunction struct{}

func (self *HumanizeFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &HumanizeArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("Humanize: %s", err.Error())
		return false
	}

	if arg.Bytes > 0 {
		return humanize.Bytes(uint64(arg.Bytes))
	}

	return fmt.Sprintf("%v", arg.Bytes)
}

func (self HumanizeFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "humanize",
		Doc:     "Format items in human readable way.",
		ArgType: type_map.AddType(&HumanizeArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&HumanizeFunction{})
}
