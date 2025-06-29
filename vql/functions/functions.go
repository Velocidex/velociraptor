/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
package functions

import (
	"bytes"
	"context"
	"encoding/ascii85"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
	"www.velocidex.com/golang/vfilter/types"
)

type _Base64DecodeArgs struct {
	String string `vfilter:"required,field=string,doc=A string to decode"`
}

type _Base64Decode struct{}

func (self _Base64Decode) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "base64decode", args)()

	arg := &_Base64DecodeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("base64decode: %s", err.Error())
		return vfilter.Null{}
	}

	result, err := base64.StdEncoding.DecodeString(arg.String)
	if err != nil {
		return vfilter.Null{}
	}
	return string(result)
}

func (self _Base64Decode) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "base64decode",
		ArgType: type_map.AddType(scope, &_Base64DecodeArgs{}),
	}
}

type _Base85Decode struct{}

func (self _Base85Decode) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "base85decode", args)()

	arg := &_Base64DecodeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("base85decode: %s", err.Error())
		return vfilter.Null{}
	}

	dest := make([]byte, len(arg.String))
	src := strings.TrimSuffix(strings.TrimPrefix(arg.String, "<~"), "~>")
	n, _, err := ascii85.Decode(dest, []byte(src), true)
	if err != nil {
		scope.Log("base85decode: %v %v", n, err)
	}
	return string(dest[:n])
}

func (self _Base85Decode) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "base85decode",
		ArgType: type_map.AddType(scope, &_Base64DecodeArgs{}),
	}
}

type _Base64EncodeArgs struct {
	String string `vfilter:"required,field=string,doc=A string to decode"`
}

type _Base64Encode struct{}

func (self _Base64Encode) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "base64encode", args)()

	arg := &_Base64EncodeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("base64encode: %s", err.Error())
		return vfilter.Null{}
	}

	result := base64.StdEncoding.EncodeToString(
		[]byte(arg.String))
	return string(result)
}

func (self _Base64Encode) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "base64encode",
		ArgType: type_map.AddType(scope, &_Base64EncodeArgs{}),
	}
}

type _ToLowerArgs struct {
	String string `vfilter:"required,field=string,doc=The string to process"`
}

type _ToLower struct{}

func (self _ToLower) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "lowcase", args)()
	arg := &_ToLowerArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("lowcase: %s", err.Error())
		return vfilter.Null{}
	}

	return strings.ToLower(arg.String)
}

func (self _ToLower) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "lowcase",
		Doc:     "Returns the lowercase version of a string.",
		ArgType: type_map.AddType(scope, &_ToLowerArgs{}),
	}
}

type _ToUpper struct{}

func (self _ToUpper) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "upcase", args)()
	arg := &_ToLowerArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upcase: %s", err.Error())
		return vfilter.Null{}
	}

	return strings.ToUpper(arg.String)
}

func (self _ToUpper) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "upcase",
		Doc:     "Returns the uppercase version of a string.",
		ArgType: type_map.AddType(scope, &_ToLowerArgs{}),
	}
}

type _ToIntArgs struct {
	String vfilter.Any `vfilter:"required,field=string,doc=A string to convert to int"`
}

type _ToInt struct{}

func (self _ToInt) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "atoi", args)()

	arg := &_ToIntArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("atoi: %s", err.Error())
		return vfilter.Null{}
	}

	switch t := arg.String.(type) {
	case string:
		result, _ := strconv.ParseInt(t, 0, 64)
		return result

	default:
		in, _ := utils.ToInt64(arg.String)
		return in
	}
}

func (self _ToInt) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "atoi",
		Doc:     "Convert a string to an int.",
		ArgType: type_map.AddType(scope, &_ToIntArgs{}),
	}
}

type _ParseFloat struct{}

func (self _ParseFloat) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "parse_float", args)()

	arg := &_ToIntArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("parse_float: %s", err.Error())
		return vfilter.Null{}
	}

	switch t := arg.String.(type) {
	case string:
		result, _ := strconv.ParseFloat(t, 64)
		return result

	case float64:
		return t

	case *float64:
		return *t

	case float32:
		return t

	case *float32:
		return *t

	default:
		in, _ := utils.ToInt64(arg.String)
		return in
	}
}

func (self _ParseFloat) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "parse_float",
		Doc:     "Convert a string to a float.",
		ArgType: type_map.AddType(scope, &_ToIntArgs{}),
	}
}

type _Now struct{}

func (self _Now) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	return time.Now().Unix()
}

func (self _Now) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "now",
		Doc:  "Returns current time in seconds since epoch.",
	}
}

type _UTF16 struct{}

func (self _UTF16) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "utf16", args)()

	arg := &_Base64DecodeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("utf16: %s", err.Error())
		return vfilter.Null{}
	}

	ints := make([]uint16, len(arg.String)/2)
	if err := binary.Read(bytes.NewReader([]byte(arg.String)), binary.LittleEndian, &ints); err != nil {
		scope.Log("utf16: %s", err.Error())
		return vfilter.Null{}
	}

	return string(utf16.Decode(ints))
}

func (self _UTF16) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "utf16",
		Doc:     "Parse input from utf16.",
		ArgType: type_map.AddType(scope, &_Base64DecodeArgs{}),
	}
}

type _UTF16Encode struct{}

func (self _UTF16Encode) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "utf16_encode", args)()

	arg := &_Base64EncodeArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("utf16_encode: %s", err.Error())
		return vfilter.Null{}
	}

	buf := bytes.NewBuffer(nil)
	ints := utf16.Encode([]rune(arg.String))
	err = binary.Write(buf, binary.LittleEndian, &ints)
	if err != nil {
		scope.Log("utf16_encode: %s", err.Error())
		return vfilter.Null{}
	}

	return buf.String()
}

func (self _UTF16Encode) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "utf16_encode",
		Doc:     "Encode a string to utf16 bytes.",
		ArgType: type_map.AddType(scope, &_Base64DecodeArgs{}),
	}
}

type _GetFunctionArgs struct {
	Item    vfilter.Any `vfilter:"optional,field=item"`
	Member  string      `vfilter:"optional,field=member"`
	Field   vfilter.Any `vfilter:"optional,field=field"`
	Default vfilter.Any `vfilter:"optional,field=default"`
}

type _GetFunction struct{}

func (self _GetFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "get",
		Doc:     "Gets the member field from item.",
		ArgType: type_map.AddType(scope, _GetFunctionArgs{}),
	}
}

func (self _GetFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "get", args)()

	arg := &_GetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("get: %s", err.Error())
		return vfilter.Null{}
	}

	if arg.Default == nil {
		arg.Default = vfilter.Null{}
	}

	result := arg.Item
	if result == nil {
		result = scope
	}

	var pres bool

	if arg.Field == nil && arg.Member == "" {
		scope.Log("get: either Field or Member should be specified.")
		return vfilter.Null{}
	}

	if arg.Field != nil {
		result, pres = scope.Associative(result, arg.Field)
		if !pres {
			return arg.Default
		}
		return result
	}

	for _, member := range strings.Split(arg.Member, ".") {
		result, pres = scope.Associative(result, member)
		if !pres {
			return arg.Default
		}
	}

	return result
}

// Allow anything to be settable.
type Setter interface {
	Set(key string, value interface{})
}

type _SetFunctionArgs struct {
	Item  vfilter.Any `vfilter:"required,field=item,doc=A dict to set"`
	Field string      `vfilter:"required,field=field,doc=The field to set"`
	Value vfilter.Any `vfilter:"required,field=value"`
}

type _SetFunction struct{}

func (self _SetFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "set",
		Doc:     "Sets the member field of the item. If item is omitted sets the scope.",
		ArgType: type_map.AddType(scope, _SetFunctionArgs{}),
	}
}

func (self _SetFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	defer vql_subsystem.RegisterMonitor(ctx, "set", args)()

	arg := &_SetFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("set: %s", err.Error())
		return vfilter.Null{}
	}

	result := arg.Item
	switch t := result.(type) {
	case types.LazyExpr:
		result = t.Reduce(ctx)
	}

	switch t := result.(type) {
	case *ordereddict.Dict:
		t.Set(arg.Field, arg.Value)
		return t

	case ordereddict.Dict:
		t.Set(arg.Field, arg.Value)
		res := ordereddict.NewDict()
		res.MergeFrom(&t)
		return res

	case map[string]interface{}:
		t[arg.Field] = arg.Value
		return t

	case Setter:
		t.Set(arg.Field, arg.Value)
		return t

	default:
		scope.Log("set: Item type %T not supported. set() expects a dict", result)
		return types.Null{}
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_Base64Decode{})
	vql_subsystem.RegisterFunction(&_Base85Decode{})
	vql_subsystem.RegisterFunction(&_Base64Encode{})
	vql_subsystem.RegisterFunction(&_SetFunction{})
	vql_subsystem.RegisterFunction(&_ToInt{})
	vql_subsystem.RegisterFunction(&_ParseFloat{})
	vql_subsystem.RegisterFunction(&_Now{})
	vql_subsystem.RegisterFunction(&_ToLower{})
	vql_subsystem.RegisterFunction(&_ToUpper{})
	vql_subsystem.RegisterFunction(&_UTF16{})
	vql_subsystem.RegisterFunction(&_UTF16Encode{})
	vql_subsystem.RegisterFunction(&_GetFunction{})
}
