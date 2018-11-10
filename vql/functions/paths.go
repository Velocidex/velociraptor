package functions

import (
	"context"
	"strings"

	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type DirnameArgs struct {
	Path string `vfilter:"required,field=path"`
}

type DirnameFunction struct{}

func (self *DirnameFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &DirnameArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("dirname: %s", err.Error())
		return false
	}

	components := utils.SplitComponents(arg.Path)
	if len(components) > 0 {
		return strings.Join(components[:len(components)-1], "/")
	}
	return vfilter.Null{}
}

func (self DirnameFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "dirname",
		Doc:     "Return the directory path.",
		ArgType: type_map.AddType(&DirnameArgs{}),
	}
}

type BasenameFunction struct{}

func (self *BasenameFunction) Call(ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &DirnameArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("basename: %s", err.Error())
		return false
	}

	components := utils.SplitComponents(arg.Path)
	if len(components) > 0 {
		return components[len(components)-1]
	}

	return vfilter.Null{}
}

func (self BasenameFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "basename",
		Doc:     "Return the basename of the path.",
		ArgType: type_map.AddType(&DirnameArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&DirnameFunction{})
	vql_subsystem.RegisterFunction(&BasenameFunction{})
}
