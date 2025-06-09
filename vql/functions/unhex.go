package functions

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UnhexFunctionArgs struct {
	String string `vfilter:"optional,field=string,doc=Hex string to decode"`
}

type UnhexFunction struct{}

func (self *UnhexFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	defer vql_subsystem.RegisterMonitor(ctx, "unhex", args)()

	arg := &UnhexFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("unhex: %s", err.Error())
		return false
	}

	// Strip all spaces
	str := strings.Replace(arg.String, " ", "", -1)
	res, _ := hex.DecodeString(strings.TrimPrefix(str, "0x"))
	return string(res)
}

func (self UnhexFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "unhex",
		Doc:     "Apply hex decoding to the string.",
		ArgType: type_map.AddType(scope, &UnhexFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UnhexFunction{})
}
