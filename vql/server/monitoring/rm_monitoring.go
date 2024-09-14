package monitoring

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RemoveClientMonitoringFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The name of the artifact to remove from the event table"`
	Label    string `vfilter:"optional,field=label,doc=Remove the artifact from this label group (default the 'all'  group)"`
}

type RemoveClientMonitoringFunction struct{}

func (self RemoveClientMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_CLIENT)
	if err != nil {
		scope.Log("rm_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &AddClientMonitoringFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("rm_client_monitoring: Command can only run on the server")
		return vfilter.Null{}
	}

	client_event_manager, err := services.ClientEventManager(config_obj)
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}
	event_config := client_event_manager.GetClientMonitoringState()

	label_config := getArtifactCollectorArgs(event_config, arg.Label)

	// First remove the current artifact if it is there already
	removeArtifact(label_config, arg.Artifact)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = client_event_manager.SetClientMonitoringState(
		ctx, config_obj, principal, event_config)
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self RemoveClientMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "rm_client_monitoring",
		Doc:      "Remove an artifact from the client monitoring table.",
		ArgType:  type_map.AddType(scope, &RemoveClientMonitoringFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_CLIENT).Build(),
	}
}

type RemoveServerMonitoringFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The name of the artifact to remove"`
}

type RemoveServerMonitoringFunction struct{}

func (self RemoveServerMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.COLLECT_SERVER)
	if err != nil {
		scope.Log("rm_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &AddServerMonitoringFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("rm_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("rm_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("rm_server_monitoring: Command can only run on the server")
		return vfilter.Null{}
	}

	server_manager, err := services.GetServerEventManager(config_obj)
	if err != nil {
		scope.Log("rm_server_monitoring: server_manager not ready")
		return vfilter.Null{}
	}

	event_config := server_manager.Get()

	// First remove the current artifact if it is there already
	removeArtifact(event_config, arg.Artifact)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = server_manager.Update(ctx, config_obj, principal, event_config)
	if err != nil {
		scope.Log("rm_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self RemoveServerMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "rm_server_monitoring",
		Doc:      "Remove an artifact from the server monitoring table.",
		ArgType:  type_map.AddType(scope, &RemoveServerMonitoringFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.COLLECT_SERVER).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RemoveClientMonitoringFunction{})
	vql_subsystem.RegisterFunction(&RemoveServerMonitoringFunction{})
}
