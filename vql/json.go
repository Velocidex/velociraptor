package vql

import (
	"context"
	"encoding/json"
	"strings"
	"www.velocidex.com/golang/vfilter"
)

type ParseJsonFunction struct{}

func (self ParseJsonFunction) Name() string {
	return "parse_json"
}

func (self ParseJsonFunction) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	data, ok := vfilter.ExtractString("data", args)
	if !ok {
		scope.Log("parse_json: Expecting a string 'data' arg")
		return &vfilter.Null{}
	}
	result := make(map[string]interface{})
	err := json.Unmarshal([]byte(*data), &result)
	if err != nil {
		scope.Log("parse_json: %s", err.Error())
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
	exportedFunctions = append(exportedFunctions, &ParseJsonFunction{})
	exportedProtocolImpl = append(exportedProtocolImpl, &_MapInterfaceAssociativeProtocol{})
}
