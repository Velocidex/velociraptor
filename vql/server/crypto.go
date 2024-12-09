package server

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type ServerFrontendCertFunction struct{}

func (self *ServerFrontendCertFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {
	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("ERROR:server_frontend_cert: %v", err)
		return vfilter.Null{}
	}

	arg := vfilter.Empty{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("ERROR:server_frontend_cert: %v", err.Error())
		return vfilter.Null{}
	}
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("ERROR:server_frontend_cert: Must be run on server")
		return vfilter.Null{}
	}

	if config_obj.Frontend == nil {
		return vfilter.Null{}
	}
	return config_obj.Frontend.Certificate
}

func (self ServerFrontendCertFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "server_frontend_cert",
		Doc:      "Get Server Frontend Certificate",
		ArgType:  type_map.AddType(scope, &vfilter.Empty{}),
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ServerFrontendCertFunction{})
}
