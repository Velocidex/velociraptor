package parsers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ParseJsonFunctionArg struct {
	Data string `vfilter:"required,field=data"`
}
type ParseJsonFunction struct{}

func (self ParseJsonFunction) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_json",
		Doc:     "Parse a JSON string into an object.",
		ArgType: type_map.AddType(scope, &ParseJsonFunctionArg{}),
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

/*
 When JSON encoding a protobuf, the output uses the original
 protobuf field names, however within Go they are converted to go
 style. For example if the protobuf has os_info, then Go fields will
 be OsInfo.

 This is very confusing to users since they first use SELECT * from
 plugin(), the * expands to Associative.GetMembers(). This should emit
 the field names that occur in the JSON. The user will then attempt to
 select such a field, and Associative() should therefore convert to
 the go style automatically.
*/
type _ProtobufAssociativeProtocol struct{}

func (self _ProtobufAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	_, b_ok := b.(string)
	if b_ok {
		switch a.(type) {
		case proto.Message, *proto.Message:
			return true
		}
	}

	return false
}

// Accept either the json emitted field name or the go style field
// name.
func (self _ProtobufAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	field, b_ok := b.(string)
	if !b_ok {
		return nil, false
	}

	if reflect.ValueOf(a).IsNil() {
		return nil, false
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	properties := proto.GetProperties(a_type)
	if properties == nil {
		return nil, false
	}

	for _, item := range properties.Prop {
		if field == item.OrigName || field == item.Name {
			result, pres := vfilter.DefaultAssociative{}.Associative(
				scope, a, item.Name)

			// If the result is an any, we decode that
			// dynamically. This is more useful than a
			// binary blob.
			any_result, ok := result.(*any.Any)
			if ok {
				var tmp_args ptypes.DynamicAny
				err := ptypes.UnmarshalAny(any_result, &tmp_args)
				if err == nil {
					return tmp_args.Message, pres
				}
			}

			return result, pres
		}
	}

	return nil, false
}

// Emit the json serializable field name only. This makes this field
// consistent with the same protobuf emitted as json using other
// means.
func (self _ProtobufAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	result := []string{}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()

	properties := proto.GetProperties(a_type)
	if properties == nil {
		return result
	}

	for _, item := range properties.Prop {
		// Only real exported fields should be collected.
		if len(item.JSONName) > 0 {
			result = append(result, item.OrigName)
		}
	}

	return result
}

func init() {
	vql_subsystem.RegisterFunction(&ParseJsonFunction{})
	vql_subsystem.RegisterProtocol(&_MapInterfaceAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_ProtobufAssociativeProtocol{})
}
