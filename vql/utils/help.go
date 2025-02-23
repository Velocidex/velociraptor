package utils

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"gopkg.in/yaml.v2"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
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

				err = yaml.Unmarshal(serialized, &info)
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
