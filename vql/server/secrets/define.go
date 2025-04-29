package secrets

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DefineSecretFunctionArgs struct {
	Type        string            `vfilter:"required,field=type,doc=Type of the secret"`
	Verifier    string            `vfilter:"optional,field=verifier,doc=A VQL Lambda function to verify the secret"`
	Description string            `vfilter:"optional,field=description,doc=A description of the secret type"`
	Template    *ordereddict.Dict `vfilter:"required,field=template,doc=A Set of key/value pairs"`
}

type DefineSecretFunction struct{}

func (self *DefineSecretFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("secret_define: %v", err)
		return vfilter.Null{}
	}

	arg := &DefineSecretFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("secret_define: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("secret_define: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("secret_define: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		scope.Log("secret_define: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)

	template := make(map[string]string)
	if arg.Template != nil {
		for _, k := range arg.Template.Keys() {
			s, pres := arg.Template.GetString(k)
			if pres {
				template[k] = s
			}
		}
	}

	err = secrets.DefineSecret(ctx, &api_proto.SecretDefinition{
		TypeName:    arg.Type,
		Verifier:    arg.Verifier,
		Description: arg.Description,
		Template:    template,
	})
	if err != nil {
		scope.Log("secret_define: %v", err)
		return vfilter.Null{}
	}

	services.LogAudit(ctx,
		org_config_obj, principal, "User Defined Secret",
		ordereddict.NewDict().
			Set("principal", principal).
			Set("type", arg.Type))

	return arg.Type
}

func (self DefineSecretFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "secret_define",
		Doc:      "Define a new secret template",
		ArgType:  type_map.AddType(scope, &DefineSecretFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&DefineSecretFunction{})
}
