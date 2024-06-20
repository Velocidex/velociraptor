package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserFunctionArgs struct {
	Username string `vfilter:"required,field=user,doc=The user to create or update."`
	OrgId    string `vfilter:"optional,field=org,doc=The org under which we query the user's ACL."`
}

type UserFunction struct{}

func (self UserFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	arg := &UserFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user: %s", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("user: %v", err)
		return vfilter.Null{}
	}

	org_config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user: Command can only run on the server")
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	users_manager := services.GetUserManager()
	user_details, err := users_manager.GetUser(ctx, principal, arg.Username)
	if err != nil {
		scope.Log("user: %s", err)
		return vfilter.Null{}
	}

	details, err := getUserRecord(ctx, scope,
		org_config_obj.OrgId, org_config_obj.OrgName, user_details)
	if err != nil {
		scope.Log("user: %v", err)
		return vfilter.Null{}
	}

	return details
}

func (self UserFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "user",
		Doc:     "Retrieves information about the Velociraptor user.",
		ArgType: type_map.AddType(scope, &UserFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&UserFunction{})
}
