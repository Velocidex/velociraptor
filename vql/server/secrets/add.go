package secrets

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

type AddSecretFunctionArgs struct {
	Name   string            `vfilter:"required,field=name,doc=Name of the secret"`
	Type   string            `vfilter:"required,field=type,doc=Type of the secret"`
	Secret *ordereddict.Dict `vfilter:"required,field=secret,doc=A Dict containing key/value pairs for the secret"`
}

type AddSecretFunction struct{}

func (self *AddSecretFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("secret_add: %v", err)
		return vfilter.Null{}
	}

	arg := &AddSecretFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("secret_add: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("secret_add: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("secret_add: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		scope.Log("secret_add: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	err = secrets.AddSecret(ctx, scope, arg.Type, arg.Name, arg.Secret)
	if err != nil {
		scope.Log("secret_add: %v", err)
		return vfilter.Null{}
	}

	return arg.Name
}

func (self AddSecretFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "secret_add",
		Doc:      "Add a new secret",
		ArgType:  type_map.AddType(scope, &AddSecretFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&AddSecretFunction{})
}
