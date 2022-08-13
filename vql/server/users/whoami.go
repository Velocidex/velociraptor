package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type WhoAmIFunctionArgs struct{}
type WhoAmIFunction struct{}

func (self *WhoAmIFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	return vql_subsystem.GetPrincipal(scope)
}

func (self WhoAmIFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "whoami",
		Doc:     "Returns the username that is running the query.",
		ArgType: type_map.AddType(scope, &WhoAmIFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&WhoAmIFunction{})
}
