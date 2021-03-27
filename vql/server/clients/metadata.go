// +build server_vql

package clients

import (
	"context"
	"errors"
	"os"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ClientMetadataFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type ClientMetadataFunction struct{}

func (self *ClientMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ClientMetadataFunctionArgs{}
	err := vfilter.ExtractArgs(scope, args, arg)
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

	client_path_manager := paths.NewClientPathManager(arg.ClientId)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("client_metadata: %s", err.Error())
		return vfilter.Null{}
	}

	result := &api_proto.ClientMetadata{}
	err = db.GetSubject(config_obj,
		client_path_manager.Metadata(), result)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		scope.Log("client_metadata: %s", err.Error())
		return vfilter.Null{}
	}

	result_dict := ordereddict.NewDict()
	for _, item := range result.Items {
		result_dict.Set(item.Key, item.Value)
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
	ClientId string `vfilter:"required,field=client_id"`
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

	permission := acls.READ_RESULTS
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
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("client_set_metadata: %s", err.Error())
		return vfilter.Null{}
	}

	result := &api_proto.ClientMetadata{ClientId: client_id}

	for _, key := range expanded_args.Keys() {
		if key == "client_id" {
			continue
		}

		value, pres := expanded_args.GetString(key)
		if !pres {
			value_any, _ := expanded_args.Get(key)
			scope.Log("client_set_metadata: metadata key %v should be a string (not type %T)",
				key, value_any)
			continue
		}

		result.Items = append(result.Items, &api_proto.ClientMetadataItem{
			Key: key, Value: value})
	}

	err = db.SetSubject(config_obj,
		client_path_manager.Metadata(), result)
	if err != nil {
		scope.Log("client_set_metadata: %s", err.Error())
		return vfilter.Null{}
	}

	return true
}

func (self ClientSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "client_set_metadata",
		Doc:     "Sets client metadata. Client metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientMetadataFunctionArgs{}),
	}
}

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
		Doc:     "Returns client metadata from the datastore. Client metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientMetadataFunctionArgs{}),
	}
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
		Doc:     "Sets client metadata. Client metadata is a set of free form key/value data",
		ArgType: type_map.AddType(scope, &ClientSetMetadataFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ClientMetadataFunction{})
	vql_subsystem.RegisterFunction(&ClientSetMetadataFunction{})
	vql_subsystem.RegisterFunction(&ServerMetadataFunction{})
	vql_subsystem.RegisterFunction(&ServerSetMetadataFunction{})
}
