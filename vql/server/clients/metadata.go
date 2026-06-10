//go:build server_vql
// +build server_vql

package clients

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ClientMetadataFunctionArgs struct {
	ClientId string `vfilter:"required,field=client_id"`
}

type ClientMetadataFunction struct {
	name string
}

func (self *ClientMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ClientMetadataFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log(self.name+": %s", err.Error())
		return vfilter.Null{}
	}

	permission := acls.READ_RESULTS
	if arg.ClientId == constants.VELOCIRAPTOR_SERVER_CLIENT_ID {
		permission = acls.SERVER_ADMIN
	}
	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log(self.name+": %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log(self.name+": %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log(self.name + ": Command can only run on the server")
		return vfilter.Null{}
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log(self.name+": %v", err)
		return vfilter.Null{}
	}

	result_dict, err := client_info_manager.GetMetadata(ctx, arg.ClientId)
	if err != nil {
		scope.Log(self.name+": %s", err)
		return vfilter.Null{}
	}

	return result_dict
}

func (self ClientMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "client_metadata",
		Doc:      "Returns client metadata from the datastore. Client metadata is a set of free form key/value data",
		ArgType:  type_map.AddType(scope, &ClientMetadataFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.READ_RESULTS, acls.SERVER_ADMIN).Build(),
	}
}

type ClientSetMetadataFunctionArgs struct {
	ClientId string            `vfilter:"required,field=client_id"`
	Metadata *ordereddict.Dict `vfilter:"optional,field=metadata,doc=A dict containing metadata. If not specified we use kwargs."`
	Modify   *vfilter.Lambda   `vfilter:"optional,field=modify,doc=A modification callback lambda. This performs an atomic mutation on the client metadata.."`
}

type ClientSetMetadataFunction struct {
	name string
}

func (self *ClientSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &ClientSetMetadataFunctionArgs{}
	kw, err := arg_parser.ExtractKWArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log(self.name+": %v", err)
		return vfilter.Null{}
	}

	if arg.Metadata == nil {
		arg.Metadata = kw
	}

	// User needs high permissions to modify the client's metadata.
	permission := acls.COLLECT_CLIENT
	if arg.ClientId == constants.VELOCIRAPTOR_SERVER_CLIENT_ID {
		permission = acls.SERVER_ADMIN
	}

	err = vql_subsystem.CheckAccess(scope, permission)
	if err != nil {
		scope.Log(self.name+": %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log(self.name+": %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log(self.name + ": Command can only run on the server")
		return vfilter.Null{}
	}

	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		scope.Log(self.name+": %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	if arg.Modify != nil {
		err = client_info_manager.ModifyMetadata(
			ctx, config_obj, arg.ClientId, principal,
			func(metadata *ordereddict.Dict) (*ordereddict.Dict, error) {
				result := arg.Modify.Reduce(
					ctx, scope, []vfilter.Any{metadata})
				dict, ok := result.(*ordereddict.Dict)
				if !ok {
					scope.Log(self.name + ": modify must return a dict")
					return nil, nil
				}
				return dict, nil
			})
	} else {
		err = client_info_manager.SetMetadata(
			ctx, arg.ClientId, arg.Metadata, principal)
	}
	if err != nil {
		scope.Log(self.name+": %s", err)
		return vfilter.Null{}
	}

	return true
}

func (self ClientSetMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:         "client_set_metadata",
		Doc:          "Sets client metadata. Client metadata is a set of free form key/value data",
		ArgType:      type_map.AddType(scope, &ClientSetMetadataFunctionArgs{}),
		Metadata:     vql.VQLMetadata().Permissions(acls.COLLECT_CLIENT, acls.SERVER_ADMIN).Build(),
		Version:      3,
		FreeFormArgs: true,
	}
}

// No args
type ServerMetadataFunctionArgs struct {
	Metadata *ordereddict.Dict `vfilter:"optional,field=metadata,doc=A dict containing metadata. If not specified we use kwargs."`
}

type ServerMetadataFunction struct{}

func (self *ServerMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	args.Set("client_id", constants.VELOCIRAPTOR_SERVER_CLIENT_ID)
	return (&ClientMetadataFunction{
		name: "server_metadata",
	}).Call(ctx, scope, args)
}

func (self ServerMetadataFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:         "server_metadata",
		Doc:          "Returns server metadata from the datastore. Server metadata is a set of free form key/value data",
		ArgType:      type_map.AddType(scope, &ServerMetadataFunctionArgs{}),
		Metadata:     vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
		Version:      3,
		FreeFormArgs: true,
	}
}

type ServerSetMetadataFunctionArgs struct {
	Metadata *ordereddict.Dict `vfilter:"optional,field=metadata,doc=A dict containing metadata. If not specified we use kwargs."`
}

type ServerSetMetadataFunction struct{}

func (self *ServerSetMetadataFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	args.Set("client_id", constants.VELOCIRAPTOR_SERVER_CLIENT_ID)
	return (&ClientSetMetadataFunction{
		name: "server_set_metadata",
	}).Call(ctx, scope, args)
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
	vql_subsystem.RegisterFunction(&ClientMetadataFunction{
		name: "client_metadata",
	})
	vql_subsystem.RegisterFunction(&ClientSetMetadataFunction{
		name: "client_set_metadata",
	})
	vql_subsystem.RegisterFunction(&ServerMetadataFunction{})
	vql_subsystem.RegisterFunction(&ServerSetMetadataFunction{})
}
