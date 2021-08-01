package filesystem

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type _RmRequest struct {
	Filename string `vfilter:"required,field=filename,doc=Filename to remove."`
}

type _RmFunction struct{}

func (self *_RmFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.FILESYSTEM_WRITE)
	if err != nil {
		scope.Log("rm: %s", err)
		return false
	}

	arg := &_RmRequest{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rm: %s", err.Error())
		return false
	}

	err = os.Remove(arg.Filename)
	if err != nil {
		scope.Log("rm: %s", err.Error())
		return false
	}

	return true
}

func (self _RmFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rm",
		Doc:     "Remove a file from the filesystem using the API.",
		ArgType: type_map.AddType(scope, &_RmRequest{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_RmFunction{})
}
