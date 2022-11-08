package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/users"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GrantFunctionArgs struct {
	Username string   `vfilter:"required,field=user,doc=The user to create or update."`
	Roles    []string `vfilter:"required,field=roles,doc=List of roles to give the user."`
	OrgIds   []string `vfilter:"optional,field=orgs,doc=One or more org IDs to grant access to."`
}

type GrantFunction struct{}

func (self GrantFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	arg := &GrantFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_grant: Command can only run on the server")
		return vfilter.Null{}
	}

	orgs := users.LIST_ALL_ORGS
	if len(arg.OrgIds) == 0 {
		orgs = []string{org_config_obj.OrgId}
	}

	policy := &acl_proto.ApiClientACL{
		Roles: arg.Roles,
	}

	principal := vql_subsystem.GetPrincipal(scope)
	err = users.AddUserToOrg(ctx, users.UseExistingUser,
		principal, arg.Username, orgs, policy)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	return arg.Username
}

func (self GrantFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "user_grant",
		Doc:     "Grants the user the specified roles.",
		ArgType: type_map.AddType(scope, &GrantFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GrantFunction{})
}
