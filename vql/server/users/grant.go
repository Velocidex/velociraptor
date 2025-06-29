package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GrantFunctionArgs struct {
	Username string            `vfilter:"required,field=user,doc=The user to create or update."`
	Roles    []string          `vfilter:"optional,field=roles,doc=List of roles to give the user."`
	OrgIds   []string          `vfilter:"optional,field=orgs,doc=One or more org IDs to grant access to. If not specified we use current org"`
	Policy   *ordereddict.Dict `vfilter:"optional,field=policy,doc=A dict of permissions to set (e.g. as obtained from the gui_users() function)."`
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

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("user_grant: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_grant: Command can only run on the server")
		return vfilter.Null{}
	}

	// If org ids not specified we use the current org id
	orgs := arg.OrgIds
	if len(arg.OrgIds) == 0 {
		orgs = []string{org_config_obj.OrgId}
	}

	policy := &acl_proto.ApiClientACL{}
	if !utils.IsNil(arg.Policy) {
		policy, err = acls.ParsePolicyFromDict(scope, arg.Policy)
		if err != nil {
			scope.Log("user_grant: %s", err)
			return vfilter.Null{}
		}
	} else if len(arg.Roles) == 0 {
		scope.Log("user_grant: You must provide either roles or a policy object")
		return vfilter.Null{}
	}
	policy.Roles = utils.DeduplicateStringSlice(append(policy.Roles, arg.Roles...))

	principal := vql_subsystem.GetPrincipal(scope)
	err = services.GrantUserToOrg(ctx, principal, arg.Username, orgs, policy)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	err = services.LogAudit(ctx,
		org_config_obj, principal, "user_grant",
		ordereddict.NewDict().
			Set("username", arg.Username).
			Set("acl", policy).
			Set("org_ids", orgs))
	if err != nil {
		logger := logging.GetLogger(org_config_obj, &logging.FrontendComponent)
		logger.Error("<red>user_grant</> %v %v %v", principal, arg.Username, policy)
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
