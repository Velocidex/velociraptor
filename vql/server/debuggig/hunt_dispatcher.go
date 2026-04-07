package debugging

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type HuntReindexArgs struct {
	HuntId string `vfilter:"optional,field=hunt_id,doc=The hunt to reindex. If not specified we index all hunts"`
}

type HuntReindex struct{}

func (self HuntReindex) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "hunt_reindex", args)()

		arg := &HuntReindexArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("hunt_reindex: %s", err.Error())
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("ERROR:hunt_reindex: Command can only run on the server")
			return
		}

		hunt_disp, err := services.GetHuntDispatcher(config_obj)
		if err != nil {
			scope.Log("ERROR:hunt_reindex: %v", err)
			return
		}

		stats, err := hunt_disp.RebuildHuntIndex(
			ctx, arg.HuntId, hunt_dispatcher.FORCE_REFRESH)
		if err != nil {
			scope.Log("ERROR:hunt_reindex: %v", err)
			return
		}

		if arg.HuntId != "" {
			hunt_obj, pres := hunt_disp.GetHunt(ctx, arg.HuntId)
			if pres {
				output_chan <- ordereddict.NewDict().
					Set("Hunt", hunt_obj).
					Set("Stats", stats)
			}

		} else {
			output_chan <- ordereddict.NewDict().
				Set("Hunt", vfilter.Null{}).
				Set("Stats", stats)
		}
	}()

	return output_chan
}

func (self HuntReindex) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "hunt_reindex",
		Doc:     "Reindex a hunt. This is mostly useful for debugging and to force an index operation out of band. Hunts will normally be reindexed periodically automatically.",
		ArgType: type_map.AddType(scope, &HuntReindexArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&HuntReindex{})
}
