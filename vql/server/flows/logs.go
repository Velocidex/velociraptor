package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type FlowLogsPluginArgs struct {
	FlowId   string `vfilter:"required,field=flow_id,doc=The flow id to read."`
	ClientId string `vfilter:"required,field=client_id,doc=The client id to extract"`
}

type FlowLogsPlugin struct{}

func (self FlowLogsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)
	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "flow_logs", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flow_logs: %s", err)
			return
		}

		arg := &FlowLogsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("flow_logs: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("flow_logs: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("flow_logs: Command can only run on the server")
			return
		}

		path_manager := paths.NewFlowPathManager(arg.ClientId, arg.FlowId)
		file_store_factory := file_store.GetFileStore(config_obj)
		rs_reader, err := result_sets.NewResultSetReader(
			file_store_factory, path_manager.Log())
		if err != nil {
			scope.Log("flow_logs: %v", err)
			return
		}

		for row := range rs_reader.Rows(ctx) {
			select {
			case <-ctx.Done():
				return
			case output_chan <- row:
			}
		}
	}()

	return output_chan
}

func (self FlowLogsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "flow_logs",
		Doc:      "Retrieve the query logs of a flow.",
		ArgType:  type_map.AddType(scope, &FlowLogsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&FlowLogsPlugin{})
}
