package parsers

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/Velocidex/yaml/v2"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	json_tools "www.velocidex.com/golang/velociraptor/tools/json"
	utils "www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
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

	// Unmarshal the YAML in such a way that we maintain the order
	// of keys.
	var parsed yaml.MapSlice
	err = yaml.Unmarshal(data, &parsed)
	if err != nil {
		scope.Log("parse_yaml: %v", err)
		return nil
	}
	result := yamlToDict(parsed)
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

func yamlToDict(item interface{}) interface{} {
	switch t := item.(type) {
	case yaml.MapSlice:
		res := ordereddict.NewDict()
		for _, v := range t {
			// We require keys to be strings since this is a JSON
			// requirement.
			key, ok := v.Key.(string)
			if !ok {
				continue
			}

			res.Set(key, yamlToDict(v.Value))
		}
		return res

	case []interface{}:
		res := []interface{}{}
		for _, v := range t {
			res = append(res, yamlToDict(v))
		}
		return res

	default:
		return item
	}
}

func (self ParseYamlFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "parse_yaml",
		Doc:      "Parse yaml into an object.",
		ArgType:  type_map.AddType(scope, &ParseYamlFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.FILESYSTEM_READ).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ParseYamlFunction{})
}
