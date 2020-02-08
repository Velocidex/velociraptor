/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package parsers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ParseJsonFunctionArg struct {
	Data string `vfilter:"required,field=data,doc=Json encoded string."`
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
	args *ordereddict.Dict) vfilter.Any {
	arg := &ParseJsonFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_json: %v", err)
		return &vfilter.Null{}
	}

	result := ordereddict.NewDict()
	err = json.Unmarshal([]byte(arg.Data), result)
	if err != nil {
		scope.Log("parse_json: %v", err)
		return &vfilter.Null{}
	}
	return result
}

type ParseJsonArray struct{}

func (self ParseJsonArray) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_json_array",
		Doc:     "Parse a JSON string into an array.",
		ArgType: type_map.AddType(scope, &ParseJsonFunctionArg{}),
	}
}

func (self ParseJsonArray) Call(
	ctx context.Context, scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	arg := &ParseJsonFunctionArg{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("parse_json_array: %v", err)
		return &vfilter.Null{}
	}

	result_array := []json.RawMessage{}
	err = json.Unmarshal([]byte(arg.Data), &result_array)
	if err != nil {
		scope.Log("parse_json_array: %v", err)
		return &vfilter.Null{}
	}

	result := make([]vfilter.Any, 0, len(result_array))
	for _, item := range result_array {
		dict := ordereddict.NewDict()
		err = json.Unmarshal(item, dict)
		if err != nil {
			// It might not be a dict - support any value.
			var any_value interface{}
			err = json.Unmarshal(item, &any_value)
			if err != nil {
				scope.Log("parse_json_array: %v", err)
				return &vfilter.Null{}
			}

			result = append(result, any_value)
			continue
		}

		result = append(result, dict)
	}

	return result
}

type ParseJsonArrayPlugin struct{}

func (self ParseJsonArrayPlugin) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		result := ParseJsonArray{}.Call(ctx, scope, args)
		result_value := reflect.Indirect(reflect.ValueOf(result))
		result_type := result_value.Type()
		if result_type.Kind() != reflect.Slice {
			for i := 0; i < result_value.Len(); i++ {
				output_chan <- result_value.Index(i).Interface()
			}
		}

	}()

	return output_chan
}

func (self ParseJsonArrayPlugin) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "parse_json_array",
		Doc:     "Parses events from a line oriented json file.",
		ArgType: type_map.AddType(scope, &ParseJsonArrayPlugin{}),
	}
}

// Associative protocol for map[string]interface{}
type _MapInterfaceAssociativeProtocol struct{}

func (self _MapInterfaceAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	if reflect.TypeOf(a).Kind() != reflect.Map {
		return false
	}

	_, b_ok := b.(string)
	return b_ok
}

func (self _MapInterfaceAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	key, key_ok := b.(string)
	map_value := reflect.ValueOf(a)
	if key_ok && map_value.Kind() == reflect.Map {
		lower_key := strings.ToLower(key)
		for _, map_key_value := range map_value.MapKeys() {
			map_key := map_key_value.String()
			// Try a case insensitive match.
			if map_key == key ||
				strings.ToLower(map_key) == lower_key {
				result := map_value.MapIndex(map_key_value)
				if !utils.IsNil(result) {
					return result.Interface(), true
				}
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
		for k := range a_map {
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

type _nilAssociativeProtocol struct{}

func (self _nilAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	value := reflect.ValueOf(a)
	return value.Kind() == reflect.Ptr && value.IsNil()
}

func (self _nilAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	return vfilter.Null{}, false
}

func (self _nilAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	return []string{}
}

// Allow a slice to be accessed by a field
type _IndexAssociativeProtocol struct{}

func (self _IndexAssociativeProtocol) Applicable(
	a vfilter.Any, b vfilter.Any) bool {

	if a == nil {
		fmt.Printf("%v\n", a)
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	a_type := a_value.Type()
	if a_type.Kind() != reflect.Slice {
		return false
	}

	switch t := b.(type) {
	case string:
		_, err := strconv.Atoi(t)
		if err == nil {
			return true
		}
	case int, float64, uint64:
		return true
	}
	return false
}

func (self _IndexAssociativeProtocol) Associative(
	scope *vfilter.Scope, a vfilter.Any, b vfilter.Any) (
	vfilter.Any, bool) {

	idx := 0
	switch t := b.(type) {
	case string:
		idx, _ = strconv.Atoi(t)
	case int:
		idx = int(t)
	case float64:
		idx = int(t)
	case uint64:
		idx = int(t)

	default:
		return vfilter.Null{}, false
	}

	a_value := reflect.Indirect(reflect.ValueOf(a))
	if a_value.Len() == 0 {
		return vfilter.Null{}, false
	}

	idx = idx % a_value.Len()

	return a_value.Index(idx).Interface(), true
}

func (self _IndexAssociativeProtocol) GetMembers(
	scope *vfilter.Scope, a vfilter.Any) []string {
	return []string{}
}

func init() {
	vql_subsystem.RegisterFunction(&ParseJsonFunction{})
	vql_subsystem.RegisterFunction(&ParseJsonArray{})
	vql_subsystem.RegisterProtocol(&_nilAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_MapInterfaceAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_ProtobufAssociativeProtocol{})
	vql_subsystem.RegisterProtocol(&_IndexAssociativeProtocol{})
	vql_subsystem.RegisterPlugin(&ParseJsonArrayPlugin{})
}
