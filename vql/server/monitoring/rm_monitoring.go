package monitoring

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type RemoveClientMonitoringFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The name of the artifact to add"`
	Label    string `vfilter:"optional,field=label,doc=Add this artifact to this label group (default all)"`
}

type RemoveClientMonitoringFunction struct{}

func (self RemoveClientMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
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

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	event_config := services.ClientEventManager().GetClientMonitoringState()

	label_config := getArtifactCollectorArgs(event_config, arg.Label)

	// First remove the current artifact if it is there already
	removeArtifact(label_config, arg.Artifact)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = services.ClientEventManager().SetClientMonitoringState(
		ctx, config_obj, principal, event_config)
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self RemoveClientMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rm_client_monitoring",
		Doc:     "Remove an artifact from the client monitoring table.",
		ArgType: type_map.AddType(scope, &AddClientMonitoringFunctionArgs{}),
	}
}

type RemoveServerMonitoringFunctionArgs struct {
	Artifact string `vfilter:"required,field=artifact,doc=The name of the artifact to add"`
}

type RemoveServerMonitoringFunction struct{}

func (self RemoveServerMonitoringFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
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

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	event_config := services.ServerEventManager.Get()

	// First remove the current artifact if it is there already
	removeArtifact(event_config, arg.Artifact)

	// Actually set the table
	principal := vql_subsystem.GetPrincipal(scope)
	err = services.ServerEventManager.Update(
		config_obj, principal, event_config)
	if err != nil {
		scope.Log("rm_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	return event_config
}

func (self RemoveServerMonitoringFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "rm_server_monitoring",
		Doc:     "Remove an artifact from the server monitoring table.",
		ArgType: type_map.AddType(scope, &AddServerMonitoringFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&RemoveClientMonitoringFunction{})
	vql_subsystem.RegisterFunction(&RemoveServerMonitoringFunction{})
}
