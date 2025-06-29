package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserCreateFunctionArgs struct {
	Username string   `vfilter:"required,field=user,doc=The user to create or update."`
	Roles    []string `vfilter:"required,field=roles,doc=List of roles to give the user."`
	Password string   `vfilter:"optional,field=password,doc=A password to set for the user (If not using SSO this might be needed)."`
	OrgIds   []string `vfilter:"optional,field=orgs,doc=One or more org IDs to grant access to. If empty we use the current org."`
}

type UserCreateFunction struct{}

func (self UserCreateFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	arg := &UserCreateFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("user_create: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_create: Command can only run on the server")
		return vfilter.Null{}
	}

	// Add the user to the current org if no orgs are specified
	if len(arg.OrgIds) == 0 {
		arg.OrgIds = append(arg.OrgIds, org_config_obj.OrgId)
	}

	principal := vql_subsystem.GetPrincipal(scope)
	policy := &acl_proto.ApiClientACL{
		Roles: arg.Roles,
	}

	users_manager := services.GetUserManager()
	err = users_manager.AddUserToOrg(ctx, services.AddNewUser,
		principal, arg.Username, arg.OrgIds, policy)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	err = services.LogAudit(ctx,
		org_config_obj, principal, "user_create",
		ordereddict.NewDict().
			Set("username", arg.Username).
			Set("acl", policy).
			Set("org_ids", arg.OrgIds))
	if err != nil {
		logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
		logger.Error("<red>user_create</> %v: %v %v", principal,
			arg.Username, policy)
	}

	if arg.Password != "" {
		// Write the user record.
		err = users_manager.SetUserPassword(
			ctx, org_config_obj, principal, arg.Username, arg.Password, "")
		if err != nil {
			scope.Log("user_create: %s", err)
			return vfilter.Null{}
		}
	}

	return arg.Username
}

func (self UserCreateFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "user_create",
		Doc:     "Creates a new user from the server, or updates their permissions or reset their password.",
		ArgType: type_map.AddType(scope, &UserCreateFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UserCreateFunction{})
}
