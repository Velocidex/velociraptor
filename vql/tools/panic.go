package tools

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func messMap(m map[string]bool) {
	for k := range m {
		m[k] = true
	}
}

func makeMapPanic() {
	m := make(map[string]bool)
	for i := 0; i < 100; i++ {
		m[fmt.Sprintf("Key %v", i)] = true
	}

	for i := 0; i < 100; i++ {
		go messMap(m)
	}
}

func init() {
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "panic",
			Metadata: vql_subsystem.VQLMetadata().
				Permissions(acls.SERVER_ADMIN).Build(),
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

				// Allow the user to select what kind of panic should be produced.
				_, pres := args.Get("MapPanic")
				if pres {
					makeMapPanic()
				}

				panic("oops")
			},
			Doc: "Crash the program with a panic!",
		})
}
