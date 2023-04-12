package functions

import (
	"context"
	"os"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

var (
	expand_regex = regexp.MustCompile("%([a-zA-Z_0-9]+)%")
)

type ExpandPathArgs struct {
	Path string `vfilter:"required,field=path,doc=A path with environment escapes"`
}

type ExpandPath struct{}

func (self ExpandPath) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("expand: %s", err)
		return vfilter.Null{}
	}

	arg := &ExpandPathArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("expand: %s", err.Error())
		return vfilter.Null{}
	}

	// Support windows style expansion on all platforms.
	return expand_env(arg.Path)
}

func expand_env(v string) string {
	return os.Expand(expand_regex.ReplaceAllString(v, "$${$1}"), getenv)
}

func getenv(v string) string {
	// Allow $ to be escaped (#850) by doubling up $
	switch v {
	case "$":
		return "$"
	}
	return os.Getenv(v)
}

func (self ExpandPath) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "expand",
		Doc:      "Expand the path using the environment.",
		ArgType:  type_map.AddType(scope, &ExpandPathArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ExpandPath{})
}
