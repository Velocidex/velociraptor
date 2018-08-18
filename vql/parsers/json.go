package parsers

import (
	"context"
	"encoding/json"
	"strings"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ParseJsonFunctionArg struct {
	Data string `vfilter:"required,field=data"`
}
type ParseJsonFunction struct{}

func (self ParseJsonFunction) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_json",
		Doc:     "Parse a JSON string into an object.",
		ArgType: type_map.AddType(&ParseJsonFunctionArg{}),
	}
}

func (self ParseJsonFunction) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &ParseJsonFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_json: %v", err)
		return &vfilter.Null{}
	}

	result := make(map[string]interface{})
	err = json.Unmarshal([]byte(arg.Data), &result)
	if err != nil {
		scope.Log("parse_json: %v", err)
		return &vfilter.Null{}
	}
	return result
}

// Associative protocol for map[string]interface{}
type _MapInterfaceAssociativeProtocol struct{}

func (self _MapInterfaceAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {
	_, a_ok := a.(map[string]interface{})
	_, b_ok := b.(string)

	return a_ok && b_ok
}

func (self _MapInterfaceAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {
	a_map, map_ok := a.(map[string]interface{})
	key, key_ok := b.(string)
	if map_ok && key_ok {
		result, pres := a_map[key]
		if pres {
			return result, true
		}

		// Try a case insensitive match.
		key = strings.ToLower(key)
		for k, v := range a_map {
			if strings.ToLower(k) == key {
				return v, true
			}
		}
	}
	return vfilter.Null{}, false
}

func (self _MapInterfaceAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	result := []string{}
	a_map, ok := a.(map[string]interface{})
	if ok {
		for k, _ := range a_map {
			result = append(result, k)
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&ParseJsonFunction{})
	vql_subsystem.RegisterProtocol(&_MapInterfaceAssociativeProtocol{})
}
