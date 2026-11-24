package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"go.yaml.in/yaml/v4"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	json_tools "www.velocidex.com/golang/velociraptor/tools/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ParseYamlFunctionArgs struct {
	Filename *accessors.OSPath `vfilter:"required,field=filename,doc=Yaml Filename"`
	Accessor string            `vfilter:"optional,field=accessor,doc=File accessor"`
	Schema   []string          `vfilter:"optional,field=schema,doc=Json schema to use for validation."`
}

type ParseYamlFunction struct{}

func (self ParseYamlFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_yaml", args)()

	arg := &ParseYamlFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_yaml: %s", err.Error())
		return nil
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}

	fd, err := accessor.OpenWithOSPath(arg.Filename)
	if err != nil {
		scope.Log("Unable to open file %s: %v",
			arg.Filename, err)
		return nil
	}
	defer fd.Close()

	data, err := utils.ReadAllWithCtx(ctx, scope, fd)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}

	// Load the YAML as nodes so mappings can be traversed in source order.
	var parsed yaml.Node
	err = yaml.Load(data, &parsed,
		yaml.WithV2Defaults(),
		yaml.WithSingleDocument())
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	result, err := yamlToDict(&parsed)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	result_dict, ok := result.(*ordereddict.Dict)
	if len(arg.Schema) == 0 || !ok {
		return result
	}

	// Validating the json requires us to pass to parse_json.
	var options json_tools.ValidationOptions
	serialized, err := json.Marshal(result)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return &vfilter.Null{}
	}

	intermediate, schema, errs := json_tools.ParseJsonToMapWithSchema(
		string(serialized), arg.Schema, options)
	if len(errs) > 0 {
		for _, err := range errs {
			scope.Log("ERROR:parse_yaml: %v", err)
		}
		return &vfilter.Null{}
	}

	json_tools.PopulateDefaults(result_dict, intermediate, schema)
	return result_dict
}

func yamlToDict(node *yaml.Node) (interface{}, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return yamlToDict(node.Content[0])

	case yaml.MappingNode:
		res := ordereddict.NewDict()
		for i := 0; i+1 < len(node.Content); i += 2 {
			// We require keys to be strings since this is a JSON
			// requirement.
			keyNode := node.Content[i]
			if keyNode.Tag != "!!str" {
				continue
			}

			value, err := yamlToDict(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			res.Set(keyNode.Value, value)
		}
		return res, nil

	case yaml.SequenceNode:
		res := []interface{}{}
		for _, child := range node.Content {
			value, err := yamlToDict(child)
			if err != nil {
				return nil, err
			}
			res = append(res, value)
		}
		return res, nil

	default:
		var result interface{}
		err := node.Load(&result, yaml.WithV2Defaults())
		return result, err
	}
}

func (self ParseYamlFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "parse_yaml",
		Doc:      "Parse yaml into an object.",
		ArgType:  type_map.AddType(scope, &ParseYamlFunctionArgs{}),
		Metadata: vql_subsystem.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
		Version:  2,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ParseYamlFunction{})
}
