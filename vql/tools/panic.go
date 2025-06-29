package tools

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "panic",
			Metadata:   vql_subsystem.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) []vfilter.Row {
				var result []vfilter.Row

				err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
				if err != nil {
					scope.Log("panic: %s", err)
					return result
				}

				panic("oops")
			},
			Doc: "Crash the program with a panic!",
		})
}
