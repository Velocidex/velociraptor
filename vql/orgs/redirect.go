package orgs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/services/frontend"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

func RedirectPluginToOrg(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	org_id string,
	plugin types.PluginGeneratorInterface,
	args *ordereddict.Dict,
) <-chan vfilter.Row {

	remote_plugin := &frontend.RelayedPlugin{
		Delegate:  plugin,
		ConfigObj: config_obj,
		Factory: func(ctx context.Context) (
			api_proto.APIClient, func() error, error) {
			return grpc_client.Factory.GetAPIClientForOrgRedirect(
				ctx, config_obj)
		}}
	return remote_plugin.Call(ctx, scope, args)

}

func RedirectFunctionToOrg(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	org_id string,
	function types.FunctionInterface,
	args *ordereddict.Dict,
) types.Any {

	remote_func := &frontend.RelayedFunction{
		Delegate:  function,
		ConfigObj: config_obj,
		Factory: func(ctx context.Context) (
			api_proto.APIClient, func() error, error) {
			return grpc_client.Factory.GetAPIClientForOrgRedirect(
				ctx, config_obj)
		}}
	return remote_func.Call(ctx, scope, args)

}
