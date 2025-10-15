package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type FlowsPluginArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
	FlowId   string `vfilter:"optional,field=flow_id"`
}

type FlowsPlugin struct{}

func (self FlowsPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "flows", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("flows: %s", err)
			return
		}

		arg := &FlowsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("flows: Command can only run on the server")
			return
		}

		client_info_manager, err := services.GetClientInfoManager(config_obj)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		// Check the client exists at all.
		_, err = client_info_manager.Get(ctx, arg.ClientId)
		if err != nil {
			scope.Log("flows: unable to get client %v: %v",
				arg.ClientId, err)
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("flows: %v", err)
			return
		}

		// The user only cares about one flow
		if arg.FlowId != "" {
			flow_details, err := launcher.GetFlowDetails(
				ctx, config_obj, services.GetFlowOptions{
					Downloads: true,
				},
				arg.ClientId, arg.FlowId)
			if err == nil {
				item := json.ConvertProtoToOrderedDict(
					flow_details.Context)
				item.Set("AvailableDownloads", flow_details.AvailableDownloads)

				select {
				case <-ctx.Done():
					return
				case output_chan <- item:
				}
			}
			return
		}

		length := int64(1000)
		offset := int64(0)

		for {
			options := result_sets.ResultSetOptions{}
			result, err := launcher.GetFlows(ctx, config_obj,
				arg.ClientId, options, offset, length)
			if err != nil {
				scope.Log("flows: %v", err)
				return
			}

			if len(result.Items) == 0 {
				return
			}

			for _, item := range result.Items {
				select {
				case <-ctx.Done():
					return
				case output_chan <- json.ConvertProtoToOrderedDict(item):
				}
			}

			offset += int64(len(result.Items))
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "flows",
		Doc:      "Retrieve the flows launched on each client.",
		ArgType:  type_map.AddType(scope, &FlowsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type CancelFlowFunction struct{}

func (self *CancelFlowFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &FlowsPluginArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("cancel_flow: %s", err.Error())
		return vfilter.Null{}
	}

	permissions := acls.COLLECT_CLIENT
	if arg.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	err = vql_subsystem.CheckAccess(scope, permissions)
	if err != nil {
		scope.Log("cancel_flow: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("cancel_flow: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("cancel_flow: Command can only run on the server")
		return vfilter.Null{}
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		scope.Log("cancel_flow: %v", err)
		return vfilter.Null{}
	}
	res, err := launcher.CancelFlow(ctx, config_obj,
		arg.ClientId, arg.FlowId, "VQL query")
	if err != nil {
		scope.Log("cancel_flow: %v", err.Error())
		return vfilter.Null{}
	}

	return json.ConvertProtoToOrderedDict(res)
}

func (self CancelFlowFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "cancel_flow",
		Doc:      "Cancels the flow.",
		ArgType:  type_map.AddType(scope, &FlowsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER, acls.COLLECT_CLIENT).Build(),
	}
}

type EnumerateFlowPlugin struct{}

func (self EnumerateFlowPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)
		defer vql_subsystem.RegisterMonitor(ctx, "enumerate_flow", args)()

		err := vql_subsystem.CheckAccess(scope, acls.READ_RESULTS)
		if err != nil {
			scope.Log("enumerate_flow: %s", err)
			return
		}

		arg := &FlowsPluginArgs{}
		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		err = services.RequireFrontend()
		if err != nil {
			scope.Log("enumerate_flow: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("enumerate_flow: Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		responses, err := launcher.Storage().DeleteFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId,
			services.NoAuditLogging, services.DryRunOnly)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		for _, resp := range responses {
			select {
			case <-ctx.Done():
				return
			case output_chan <- resp:
			}
		}
	}()

	return output_chan
}

func (self EnumerateFlowPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:     "enumerate_flow",
		Doc:      "Enumerate all the files that make up a flow.",
		ArgType:  type_map.AddType(scope, &FlowsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS).Build(),
	}
}

type GetFlowFunction struct{}

func (self *GetFlowFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &FlowsPluginArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("get_flow: %s", err.Error())
		return vfilter.Null{}
	}

	permissions := acls.COLLECT_CLIENT
	if arg.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	err = vql_subsystem.CheckAccess(scope, permissions)
	if err != nil {
		scope.Log("get_flow: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("get_flow: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("get_flow: Command can only run on the server")
		return vfilter.Null{}
	}

	launcher, err := services.GetLauncher(config_obj)
	if err != nil {
		scope.Log("get_flow: %v", err)
		return vfilter.Null{}
	}
	res, err := launcher.GetFlowDetails(
		ctx, config_obj, services.GetFlowOptions{
			Downloads: true,
		},
		arg.ClientId, arg.FlowId)
	if err != nil {
		scope.Log("get_flow: %v", err)
		return vfilter.Null{}
	}

	return json.ConvertProtoToOrderedDict(res.Context).
		Set("AvailableDownloads", res.AvailableDownloads)
}

func (self GetFlowFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "get_flow",
		Doc:      "Gets flow details.",
		ArgType:  type_map.AddType(scope, &FlowsPluginArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_CLIENT, acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&EnumerateFlowPlugin{})
	vql_subsystem.RegisterFunction(&CancelFlowFunction{})
	vql_subsystem.RegisterFunction(&GetFlowFunction{})
	vql_subsystem.RegisterPlugin(&FlowsPlugin{})
}
