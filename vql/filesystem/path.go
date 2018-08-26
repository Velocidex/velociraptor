package filesystem

import (
	"context"
	"regexp"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var basename_regexp = regexp.MustCompile("([^/\\\\]+)$")

type _BasenameArgs struct {
	Path string `vfilter:"required,field=path"`
}

type _Basename struct{}

func (self _Basename) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_BasenameArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("basename: %s", err.Error())
		return vfilter.Null{}
	}

	match := basename_regexp.FindStringSubmatch(arg.Path)
	if match != nil {
		return match[1]
	}
	return "/"
}

func (self _Basename) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "basename",
		Doc:     "Splits the path on separator and return the basename.",
		ArgType: type_map.AddType(&_BasenameArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_Basename{})
}
