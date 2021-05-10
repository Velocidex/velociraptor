package parsers

import (
	"context"
	"io/ioutil"

	"github.com/Velocidex/ordereddict"
	"sigs.k8s.io/yaml"
	"www.velocidex.com/golang/velociraptor/glob"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ParseYamlFunctionArgs struct {
	Filename string `vfilter:"required,field=filename,doc=Yaml Filename"`
	Accessor string `vfilter:"optional,field=accessor,doc=File accessor`
}

type ParseYamlFunction struct{}

func (self ParseYamlFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ParseYamlFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_yaml: %s", err.Error())
		return nil
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("parse_yaml: %s", err)
		return nil
	}

	accessor, err := glob.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}

	fd, err := accessor.Open(arg.Filename)
	if err != nil {
		scope.Log("Unable to open file %s: %v",
			arg.Filename, err)
		return nil
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	m, err := yaml.YAMLToJSON(data)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	ordered_dict := ordereddict.NewDict()
	err = ordered_dict.UnmarshalJSON(m)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	return ordered_dict

}

func (self ParseYamlFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_yaml",
		Doc:     "Parse yaml into an object.",
		ArgType: type_map.AddType(scope, &ParseYamlFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ParseYamlFunction{})
}
