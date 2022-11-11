// +build server_vql

package clients

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ClientMetadataFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type ClientMetadataFunction struct{}

func (self *ClientMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ClientMetadataFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("client_metadata: %s", err.Error())
		return vfilter.Null{}
	}

	permission := acls.READ_RESULTS
	if arg.ClientId == "server" {
		permission = acls.SERVER_ADMIN
	}
	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("client_metadata: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log("client_metadata: %s", err)
		return vfilter.Null{}
	}

	result_dict, err := client_info_manager.GetMetadata(ctx, arg.ClientId)
	if err != nil {
		scope.Log("client_metadata: %s", err)
		return vfilter.Null{}
	}

	return result_dict
}

func (self ClientMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "client_metadata",
		Doc:     "Returns client metadata from the datastore. Client metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientMetadataFunctionArgs{}),
	}
}

type ClientSetMetadataFunctionArgs struct {
	ClientId string            `vfilter:"required,field=client_id"`
	Metadata *ordereddict.Dict `vfilter:"optional,field=metadata,doc=A dict containing metadata. If not specified we use kwargs."`
}

type ClientSetMetadataFunction struct{}

func (self *ClientSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// Collapse lazy args etc.
	expanded_args := vfilter.RowToDict(ctx, scope, args)
	client_id, pres := expanded_args.GetString("client_id")
	if !pres {
		scope.Log("client_set_metadata: client_id must be specified")
		return vfilter.Null{}
	}

	// Allow the metadata to be set.
	metadata_any, pres := expanded_args.Get("metadata")
	if pres {
		metadata := vfilter.RowToDict(ctx, scope, metadata_any)
		if metadata != nil {
			expanded_args.MergeFrom(metadata)
		}
	}

	// User needs high permissions to modify the client's metadata.
	permission := acls.COLLECT_SERVER
	if client_id == "server" {
		permission = acls.SERVER_ADMIN
	}

	err := vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log("client_set_metadata: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("client_set_metadata: Command can only run on the server")
		return vfilter.Null{}
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log("client_set_metadata: %s", err)
		return vfilter.Null{}
	}

	err = client_info_manager.SetMetadata(ctx, client_id, expanded_args)
	if err != nil {
		scope.Log("client_set_metadata: %s", err)
		return vfilter.Null{}
	}

	return true
}

func (self ClientSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "client_set_metadata",
		Doc:     "Sets client metadata. Client metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientSetMetadataFunctionArgs{}),
	}
}

// No args
type ServerMetadataFunctionArgs struct{}

type ServerMetadataFunction struct{}

func (self *ServerMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	args.Set("client_id", "server")
	return (&ClientMetadataFunction{}).Call(ctx, scope, args)
}

func (self ServerMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "server_metadata",
		Doc:     "Returns server metadata from the datastore. Server metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientMetadataFunctionArgs{}),
	}
}

type ServerSetMetadataFunctionArgs struct {
	Metadata *ordereddict.Dict `vfilter:"optional,field=metadata,doc=A dict containing metadata. If not specified we use kwargs."`
}

type ServerSetMetadataFunction struct{}

func (self *ServerSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	args.Set("client_id", "server")
	return (&ClientSetMetadataFunction{}).Call(ctx, scope, args)
}

func (self ServerSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "server_set_metadata",
		Doc:     "Sets server metadata. Server metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ServerSetMetadataFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ClientMetadataFunction{})
	vql_subsystem.RegisterFunction(&ClientSetMetadataFunction{})
	vql_subsystem.RegisterFunction(&ServerMetadataFunction{})
	vql_subsystem.RegisterFunction(&ServerSetMetadataFunction{})
}
