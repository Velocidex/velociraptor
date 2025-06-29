package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserDeleteFunctionArgs struct {
	Username   string   `vfilter:"required,field=user,doc=The user to delete."`
	OrgIds     []string `vfilter:"optional,field=orgs,doc=If set we only delete from these orgs, otherwise we delete from the current org."`
	ReallyDoIt bool     `vfilter:"optional,field=really_do_it,doc=If not specified, just show what user will be removed"`
}

type UserDeleteFunction struct{}

func (self UserDeleteFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := services.RequireFrontend()
	if err != nil {
		scope.Log("user_delete: %v", err)
		return vfilter.Null{}
	}

	// ACLs are checked by the users module
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_delete, Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &UserDeleteFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	orgs := []string{config_obj.OrgId}
	if len(arg.OrgIds) != 0 {
		orgs = arg.OrgIds
	}

	if arg.ReallyDoIt {
		principal := vql_subsystem.GetPrincipal(scope)
		users_manager := services.GetUserManager()
		err = users_manager.DeleteUser(ctx, principal, arg.Username, orgs)
		if err != nil {
			scope.Log("user_delete: %s", err)
			return vfilter.Null{}
		}

		err = services.LogAudit(ctx,
			config_obj, principal, "user_delete",
			ordereddict.NewDict().
				Set("username", arg.Username).
				Set("org_ids", orgs))
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("<red>user_delete</> %v %v", principal, arg.Username)
		}

	} else {
		scope.Log("user_delete: Will remove %v from orgs %v", arg.Username, orgs)
	}

	return arg.Username
}

func (self UserDeleteFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "user_delete",
		Doc:     "Deletes a user from the server.",
		ArgType: type_map.AddType(scope, &UserDeleteFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UserDeleteFunction{})
}
