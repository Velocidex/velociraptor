// +build !windows

// This is the non windows version. We only support go style
// expansions (i.e. $Temp - on non windows systems we do not support
// windows style expands (e.g. %TEMP%)
package functions

import (
	"context"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type _ExpandPathArgs struct {
	Path string `vfilter:"required,field=path,doc=A path with environment escapes"`
}

type _ExpandPath struct{}

func (self _ExpandPath) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("expand: %s", err)
		return vfilter.Null{}
	}

	arg := &_ExpandPathArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("expand: %s", err.Error())
		return vfilter.Null{}
	}

	return os.Expand(arg.Path, getenv)
}

func getenv(v string) string {
	// Allow $ to be escaped (#850) by doubling up $
	if v == "$" {
		return "$"
	}
	return os.Getenv(v)
}

func (self _ExpandPath) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "expand",
		Doc:     "Expand the path using the environment.",
		ArgType: type_map.AddType(scope, &_ExpandPathArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_ExpandPath{})
}
