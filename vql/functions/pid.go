package functions

import (
	"context"
	"os"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GetPidFunction struct{}

func (self *GetPidFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {

	return os.Getpid()
}

func (self GetPidFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "getpid",
		Doc:  "Returns the current pid of the process.",
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GetPidFunction{})
}
