package functions

import (
	"context"
	"strconv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	vfilter "www.velocidex.com/golang/vfilter"
)

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
		scope.Log("%s: %s", self.Name(), err.Error())
		return vfilter.Null{}
	}

	result, _ := strconv.Atoi(arg.String)
	return result
}

func (self _ToInt) Name() string {
	return "atoi"
}

func init() {
	vql_subsystem.RegisterFunction(&_ToInt{})
}
