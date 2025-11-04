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

type ModifySecretFunctionArgs struct {
	Name             string   `vfilter:"required,field=name,doc=Name of the secret"`
	Type             string   `vfilter:"required,field=type,doc=Type of the secret"`
	Delete           bool     `vfilter:"optional,field=delete,doc=Delete the secret completely"`
	AddUsers         []string `vfilter:"optional,field=add_users,doc=A list of users to add to the secret"`
	RemoveUsers      []string `vfilter:"optional,field=remove_users,doc=A list of users to remove from the secret"`
	VisibleToAllOrgs bool     `vfilter:"optional,field=visible_to_all_orgs,doc=If set we make the secret visible to all orgs"`
	AddOrgs          []string `vfilter:"optional,field=add_orgs,doc=A list of orgs to add to the secret"`
	RemoveOrgs       []string `vfilter:"optional,field=remove_orgs,doc=A list of orgs to remove from the secret"`
}

type ModifySecretFunction struct{}

func (self *ModifySecretFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("secret_modify: %v", err)
		return vfilter.Null{}
	}

	arg := &ModifySecretFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("secret_modify: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("secret_modify: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("secret_modify: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	secrets, err := services.GetSecretsService(org_config_obj)
	if err != nil {
		scope.Log("secret_modify: Command can only run on the server: %v", err)
		return vfilter.Null{}
	}

	err = secrets.ModifySecret(ctx, &api_proto.ModifySecretRequest{
		Name:             arg.Name,
		TypeName:         arg.Type,
		Delete:           arg.Delete,
		AddUsers:         arg.AddUsers,
		RemoveUsers:      arg.RemoveUsers,
		AddOrgs:          arg.AddOrgs,
		RemoveOrgs:       arg.RemoveOrgs,
		VisibleToAllOrgs: arg.VisibleToAllOrgs,
	})
	if err != nil {
		scope.Log("secret_modify: %v", err)
		return vfilter.Null{}
	}

	return arg.Name
}

func (self ModifySecretFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "secret_modify",
		Doc:      "Modify the secret",
		ArgType:  type_map.AddType(scope, &ModifySecretFunctionArgs{}),
		Metadata: vql.VQLMetadata().Permissions(acls.SERVER_ADMIN).Build(),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&ModifySecretFunction{})
}
