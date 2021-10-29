package monitoring

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GetClientMonitoringArgs struct{}

type GetClientMonitoring struct{}

func (self GetClientMonitoring) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("get_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &GetClientMonitoringArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("get_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	_, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	return services.ClientEventManager().GetClientMonitoringState()
}

func (self GetClientMonitoring) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "get_client_monitoring",
		Doc:     "Retrieve the current client monitoring state.",
		ArgType: type_map.AddType(scope, &GetClientMonitoringArgs{}),
	}
}

type SetClientMonitoringArgs struct {
	Data vfilter.Any `vfilter:"required,field=value,doc=The Value to set"`
}

type SetClientMonitoring struct{}

func (self SetClientMonitoring) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("set_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &SetClientMonitoringArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("set_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	value_json := ""
	switch t := arg.Data.(type) {
	case string:
		value_json = t

	default:
		opts := vql_subsystem.EncOptsFromScope(scope)
		serialized, err := json.MarshalWithOptions(arg.Data, opts)
		if err != nil {
			scope.Log("set_client_monitoring: %v", err)
			return vfilter.Null{}
		}
		value_json = string(serialized)
	}

	// This should also validate the json.
	value := &flows_proto.ClientEventTable{}
	err = json.Unmarshal([]byte(value_json), value)
	if err != nil {
		scope.Log("set_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = services.ClientEventManager().SetClientMonitoringState(
		ctx, config_obj, principal, value)
	if err != nil {
		scope.Log("set_client_monitoring: %s", err.Error())
		return vfilter.Null{}
	}

	return value
}

func (self SetClientMonitoring) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "set_client_monitoring",
		Doc:     "Sets the current client monitoring state.",
		ArgType: type_map.AddType(scope, &SetClientMonitoringArgs{}),
	}
}

type GetServerMonitoringArgs struct{}

type GetServerMonitoring struct{}

func (self GetServerMonitoring) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("get_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &GetServerMonitoringArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("get_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("get_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	result := &flows_proto.ArtifactCollectorArgs{}
	err = db.GetSubject(config_obj,
		paths.ServerMonitoringFlowURN,
		result)

	if err != nil {
		scope.Log("get_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	return result
}

func (self GetServerMonitoring) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "get_server_monitoring",
		Doc:     "Retrieve the current client monitoring state.",
		ArgType: type_map.AddType(scope, &GetServerMonitoringArgs{}),
	}
}

type SetServerMonitoringArgs struct {
	Data vfilter.Any `vfilter:"required,field=value,doc=The Value to set"`
}

type SetServerMonitoring struct{}

func (self SetServerMonitoring) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("set_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &SetServerMonitoringArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("set_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	value_json := ""
	switch t := arg.Data.(type) {
	case string:
		value_json = t

	default:
		opts := vql_subsystem.EncOptsFromScope(scope)
		serialized, err := json.MarshalWithOptions(arg.Data, opts)
		if err != nil {
			scope.Log("set_server_monitoring: %v", err)
			return vfilter.Null{}
		}
		value_json = string(serialized)
	}

	// This should also validate the json.
	value := &flows_proto.ArtifactCollectorArgs{}
	err = json.Unmarshal([]byte(value_json), value)
	if err != nil {
		scope.Log("set_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	server_manager := services.GetServerEventManager()
	if server_manager == nil {
		scope.Log("set_server_monitoring: server_manager not ready")
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = server_manager.Update(config_obj, principal, value)
	if err != nil {
		scope.Log("set_server_monitoring: %s", err.Error())
		return vfilter.Null{}
	}

	return value
}

func (self SetServerMonitoring) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "set_server_monitoring",
		Doc:     "Sets the current server monitoring state (this function is deprecated, use add_server_monitoring and remove_server_monitoring).",
		ArgType: type_map.AddType(scope, &SetServerMonitoringArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GetClientMonitoring{})
	vql_subsystem.RegisterFunction(&SetClientMonitoring{})
	vql_subsystem.RegisterFunction(&GetServerMonitoring{})
	vql_subsystem.RegisterFunction(&SetServerMonitoring{})
}
