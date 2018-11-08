package functions

import (
	"context"
	"encoding/base64"
	"strconv"
	"time"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

type _Base64DecodeArgs struct {
	String string `vfilter:"required,field=string"`
}

type _Base64Decode struct{}

func (self _Base64Decode) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_Base64DecodeArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

func (self _Base64Decode) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "base64decode",
		ArgType: type_map.AddType(&_Base64DecodeArgs{}),
	}
}

type _ToIntArgs struct {
	String string `vfilter:"required,field=string"`
}

type _ToInt struct{}

func (self _ToInt) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	arg := &_ToIntArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("atoi: %s", err.Error())
		return vfilter.Null{}
	}

	result, _ := strconv.Atoi(arg.String)
	return result
}

func (self _ToInt) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "atoi",
		Doc:     "Convert a string to an int.",
		ArgType: type_map.AddType(&_ToIntArgs{}),
	}
}

type _Now struct{}

func (self _Now) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *vfilter.Dict) vfilter.Any {
	return time.Now().Unix()
}

func (self _Now) Info(type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "now",
		Doc:     "Returns current time in seconds since epoch.",
		ArgType: type_map.AddType(&_ToIntArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&_ToInt{})
	vql_subsystem.RegisterFunction(&_Now{})
}
