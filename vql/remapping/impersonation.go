package remapping

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

var (
	expand_regex = regexp.MustCompile("%([a-zA-Z0-9]+)%")
)

type ExpandPathArgs struct {
	Path string `vfilter:"required,field=path,doc=A path with environment escapes"`
}

type ImpersonatedExpand struct {
	Env *ordereddict.Dict
}

func (self ImpersonatedExpand) Copy() types.FunctionInterface {
	return self
}

func (self ImpersonatedExpand) Call(
	ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "expand", args)()

	arg := &ExpandPathArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("expand: %s", err.Error())
		return vfilter.Null{}
	}

	// Convert the string from windows standard to go standard.
	arg.Path = expand_regex.ReplaceAllString(arg.Path, "$${$1}")
	return os.Expand(arg.Path, self.getenv)
}

func (self ImpersonatedExpand) getenv(v string) string {
	// Allow $ to be escaped (#850) by doubling up $
	if v == "$" {
		return "$"
	}

	value, _ := self.Env.GetString(strings.ToLower(v))
	return value
}

func (self ImpersonatedExpand) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "expand",
		Doc:     "Expand the path using the environment.",
		ArgType: type_map.AddType(scope, &ExpandPathArgs{}),
	}
}

func installExpandMock(
	scope vfilter.Scope, remappings []*config_proto.RemappingConfig) {
	mock_env := ordereddict.NewDict()
	for _, remapping := range remappings {
		if remapping.Type == "impersonation" {
			for _, env := range remapping.Env {
				mock_env.Set(strings.ToLower(env.Key), env.Value)
			}
		}
	}

	scope.AppendFunctions(&ImpersonatedExpand{Env: mock_env})
}

type DisabledPlugin struct {
	name string
}

func (self *DisabledPlugin) Info(scope types.Scope,
	type_map *types.TypeMap) *types.PluginInfo {
	return &types.PluginInfo{
		Name: self.name,
	}
}

func (self *DisabledPlugin) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) <-chan types.Row {
	output_chan := make(chan types.Row)
	go func() {
		defer close(output_chan)

		functions.DeduplicatedLog(ctx, scope, "Call to plugin %v disabled", self.name)
	}()
	return output_chan
}

type DisabledFunction struct {
	name string
}

func (self *DisabledFunction) Info(scope types.Scope,
	type_map *types.TypeMap) *types.FunctionInfo {
	return &types.FunctionInfo{
		Name: self.name,
	}
}

func (self *DisabledFunction) Copy() types.FunctionInterface {
	return &DisabledFunction{
		name: self.name,
	}
}

func (self *DisabledFunction) Call(ctx context.Context,
	scope types.Scope, args *ordereddict.Dict) types.Any {
	functions.DeduplicatedLog(ctx, scope, "Call to function %v disabled", self.name)
	return vfilter.Null{}
}

func disablePlugins(
	remapped_scope vfilter.Scope,
	remapping *config_proto.RemappingConfig) {

	for _, pl := range remapping.DisabledPlugins {
		remapped_scope.AppendPlugins(&DisabledPlugin{name: pl})
	}

	for _, pl := range remapping.DisabledFunctions {
		remapped_scope.AppendFunctions(&DisabledFunction{name: pl})
	}
}
