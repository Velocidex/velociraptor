package golang

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterFunction(
		vfilter.GenericFunction{
			FunctionName: "typeof",
			Doc: `Print the underlying Go type of the variable.

You can use any Keyword arg, the first one will be returned.
`,
			Metadata: vql_subsystem.VQLMetadata().Build(),
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) vfilter.Any {

				for _, k := range args.Keys() {
					v, _ := args.Get(k)
					return fmt.Sprintf("%T", vql_subsystem.Materialize(ctx, scope, v))
				}

				return &vfilter.Null{}
			},
		})
}
