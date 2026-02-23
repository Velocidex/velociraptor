package functions

import (
	"context"
	"strings"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ReformatFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The artifact VQL to reformat."`
}

type ReformatFunction struct{}

type ReformatFunctionResult struct {
	Artifact string
	Error    string
}

func (self *ReformatFunctionResult) ToDict() *ordereddict.Dict {
	return ordereddict.NewDict().
		Set("Artifact", self.Artifact).
		Set("Error", self.Error)
}

func (self *ReformatFunction) Call(ctx context.Context, scope vfilter.Scope, args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "reformat", args)()

	result := &ReformatFunctionResult{}

	arg := &ReformatFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		result.Artifact = arg.Artifact
		result.Error = err.Error()
		return result.ToDict()
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("reformat: Must be run on the server")
		return vfilter.Null{}
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		result.Artifact = arg.Artifact
		result.Error = err.Error()
		return result.ToDict()
	}

	reformatted, err := manager.ReformatVQL(ctx, arg.Artifact)
	if err != nil {
		result.Artifact = arg.Artifact
		result.Error = err.Error()
		return result.ToDict()
	}

	result.Artifact = strings.Trim(reformatted, "\n")
	result.Error = ""

	return result.ToDict()
}

func (self ReformatFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "reformat",
		Doc: `Reformat VQL

This function will reformat the artifact provided and return the reformatted content.`,
		ArgType: type_map.AddType(scope, &ReformatFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ReformatFunction{})
}
