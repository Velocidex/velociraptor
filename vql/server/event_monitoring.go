package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/ptypes/empty"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/artifacts"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type GetClientMonitoringArgs struct{}

type GetClientMonitoring struct{}

func (self GetClientMonitoring) Call(
	ctx context.Context,
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("get_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &GetClientMonitoringArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("get_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	_, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	return services.ClientEventManager().GetClientMonitoringState()
}

func (self GetClientMonitoring) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("set_client_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &SetClientMonitoringArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("set_client_monitoring: %v", err)
		return vfilter.Null{}
	}

	_, ok := artifacts.GetServerConfig(scope)
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

	err = services.ClientEventManager().SetClientMonitoringState(value)
	if err != nil {
		scope.Log("set_client_monitoring: %s", err.Error())
		return vfilter.Null{}
	}

	return value
}

func (self SetClientMonitoring) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("get_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &GetServerMonitoringArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("get_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	if err != nil {
		scope.Log("get_server_monitoring: %s", err.Error())
		return vfilter.Null{}
	}
	defer closer()

	response, err := client.GetServerMonitoringState(ctx, &empty.Empty{})
	if err != nil {
		scope.Log("get_server_monitoring: %s", err.Error())
		return vfilter.Null{}
	}

	return response
}

func (self GetServerMonitoring) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
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
	scope *vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("set_server_monitoring: %s", err)
		return vfilter.Null{}
	}

	arg := &SetServerMonitoringArgs{}
	err = vfilter.ExtractArgs(scope, args, arg)
	if err != nil {
		scope.Log("set_server_monitoring: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := artifacts.GetServerConfig(scope)
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

	client, closer, err := grpc_client.Factory.GetAPIClient(ctx, config_obj)
	if err != nil {
		scope.Log("set_server_monitoring: %s", err.Error())
		return vfilter.Null{}
	}
	defer closer()

	response, err := client.SetServerMonitoringState(ctx, value)
	if err != nil {
		scope.Log("set_server_monitoring: %s", err.Error())
		return vfilter.Null{}
	}

	return response
}

func (self SetServerMonitoring) Info(scope *vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "set_server_monitoring",
		Doc:     "Sets the current server monitoring state.",
		ArgType: type_map.AddType(scope, &SetServerMonitoringArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GetClientMonitoring{})
	vql_subsystem.RegisterFunction(&SetClientMonitoring{})
	vql_subsystem.RegisterFunction(&GetServerMonitoring{})
	vql_subsystem.RegisterFunction(&SetServerMonitoring{})
}
