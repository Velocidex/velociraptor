// +build server_vql

package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
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
				config_obj, arg.ClientId, arg.FlowId)
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

		length := uint64(1000)
		offset := uint64(0)

		for {
			result, err := launcher.GetFlows(config_obj,
				arg.ClientId, true, nil, offset, length)
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

			offset += uint64(len(result.Items))
		}
	}()

	return output_chan
}

func (self FlowsPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flows",
		Doc:     "Retrieve the flows launched on each client.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
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

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
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
		Name:    "cancel_flow",
		Doc:     "Cancels the flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
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

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("delete_flow: %v", err)
			return
		}

		responses, err := launcher.DeleteFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId, false /* really_do_it */)
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
		Name:    "enumerate_flow",
		Doc:     "Enumerate all the files that make up a flow.",
		ArgType: type_map.AddType(scope, &FlowsPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&EnumerateFlowPlugin{})
	vql_subsystem.RegisterFunction(&CancelFlowFunction{})
	vql_subsystem.RegisterPlugin(&FlowsPlugin{})
}
