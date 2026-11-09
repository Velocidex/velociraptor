package utils

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

func init() {
	vql_subsystem.RegisterPlugin(
		vfilter.GenericListPlugin{
			PluginName: "help",
			Function: func(
				ctx context.Context,
				scope vfilter.Scope,
				args *ordereddict.Dict) (res []vfilter.Row) {

				var info []*api_proto.Completion

				serialized, err := assets.ReadFile("/docs/references/vql.yaml")
				if err != nil {
					scope.Log("help: %v", err)
					return nil
				}

				err = utils.YamlUnmarshalStrict(serialized, &info)
				if err != nil {
					scope.Log("help: %v", err)
					return nil
				}

				for _, i := range info {
					res = append(res, i)
				}

				return res
			},
			Doc: "Dump information about all VQL functions and plugins.",
		})
}
