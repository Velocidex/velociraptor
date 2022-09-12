package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
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

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	arg := &GrantFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUserWithHashes(ctx, arg.Username)
	if err != nil {
		scope.Log("user_grant: %s", err)
		return vfilter.Null{}
	}

	// Grat the user the roles in all orgs.
	org_config_obj := config_obj

	// No OrgIds specified - the user will be created in the root org.
	if len(arg.OrgIds) == 0 {
		// Grant the roles to the user
		err = acls.GrantRoles(config_obj, arg.Username, arg.Roles)
		if err != nil {
			scope.Log("user_grant: %s", err)
			return vfilter.Null{}
		}

		// OrgIds specified, grant the user an ACL in each org specified.
	} else {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("user_grant: %v", err)
			return vfilter.Null{}
		}

		for _, org_id := range arg.OrgIds {
			org_config_obj, err = org_manager.GetOrgConfig(org_id)
			if err != nil {
				scope.Log("user_grant: %v", err)
				return vfilter.Null{}
			}

			// Grant the roles to the user
			err = acls.GrantRoles(org_config_obj, arg.Username, arg.Roles)
			if err != nil {
				scope.Log("user_grant: %s", err)
				return vfilter.Null{}
			}
		}

		org_exists := func(org_id string) bool {
			for _, org := range user_record.Orgs {
				if org.Id == org_id {
					return true
				}
			}
			return false
		}

		for _, org_id := range arg.OrgIds {
			if !org_exists(org_id) {
				user_record.Orgs = append(user_record.Orgs,
					&api_proto.Org{Id: org_id})
			}
		}
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
