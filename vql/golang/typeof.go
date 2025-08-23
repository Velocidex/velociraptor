package golang

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type TypeOfFunction struct{}

func (self TypeOfFunction) Call(ctx context.Context,
	scope vfilter.Scope, args *ordereddict.Dict) vfilter.Any {
	for _, v := range args.Values() {
		return fmt.Sprintf("%T", vql_subsystem.Materialize(ctx, scope, v))
	}

	return &vfilter.Null{}
}

func (self TypeOfFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "typeof",
		Doc: `Print the underlying Go type of the variable.

You can use any Keyword arg, the first one will be returned.
`,
		Metadata:     vql_subsystem.VQLMetadata().Build(),
		FreeFormArgs: true,
	}
}

func init() {
	vql_subsystem.RegisterFunction(&TypeOfFunction{})
}
