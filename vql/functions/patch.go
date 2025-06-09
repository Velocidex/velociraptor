package functions

import (
	"context"

	"github.com/Velocidex/ordereddict"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type PatchFunctionArgs struct {
	Item  vfilter.Any `vfilter:"required,field=item,doc=The item to patch"`
	Patch vfilter.Any `vfilter:"optional,field=patch,doc=A JSON patch to apply"`
	Merge vfilter.Any `vfilter:"optional,field=merge,doc=A merge patch to apply"`
}

type PatchFunction struct{}

func (self PatchFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "patch",
		Doc:     "Patch a JSON object with a JSON patch.",
		ArgType: type_map.AddType(scope, PatchFunctionArgs{}),
	}
}

func to_json(item vfilter.Any) ([]byte, error) {
	switch t := item.(type) {
	case string:
		return []byte(t), nil

	case []byte:
		return t, nil

	default:
		return json.Marshal(item)
	}
}

func (self *PatchFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &PatchFunctionArgs{}

	defer vql_subsystem.RegisterMonitor(ctx, "patch", args)()

	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("patch: %s", err.Error())
		return vfilter.Null{}
	}

	value_str, err := to_json(arg.Item)
	if err != nil {
		scope.Log("patch: %v", err)
		return vfilter.Null{}
	}

	var patched []byte
	if arg.Merge != nil {
		merge, err := to_json(arg.Merge)
		if err != nil {
			scope.Log("patch: %v", err)
			return vfilter.Null{}
		}

		patched, err = jsonpatch.MergePatch(value_str, merge)
		if err != nil {
			scope.Log("patch: %v", err)
			return vfilter.Null{}
		}
	} else if arg.Patch != nil {
		patch_str, err := to_json(arg.Patch)
		if err != nil || len(patch_str) < 2 {
			scope.Log("patch: %v", err)
			return vfilter.Null{}
		}

		// If the patch is an object we convert it to a list
		// of objects.
		if patch_str[0] == '{' {
			patch_str = append([]byte{'['}, patch_str...)
			patch_str = append(patch_str, ']')
		}

		patch, err := jsonpatch.DecodePatch(patch_str)
		if err != nil {
			scope.Log("patch: %v", err)
			return vfilter.Null{}
		}

		patched, err = patch.Apply([]byte(value_str))
		if err != nil {
			scope.Log("patch: %v", err)
			return vfilter.Null{}
		}
	} else {
		scope.Log("Either patch or merge must be provided.")
		return vfilter.Null{}
	}

	item, err := utils.ParseJsonToObject(patched)
	if err != nil {
		scope.Log("patch: %v", err)
		return vfilter.Null{}
	}

	return item
}

func init() {
	vql_subsystem.RegisterFunction(&PatchFunction{})
}
