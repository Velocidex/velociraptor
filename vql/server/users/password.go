package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type SetPasswordFunctionArgs struct {
	Username string `vfilter:"optional,field=user,doc=The user to set password for. If not set, changes the current user's password."`
	Password string `vfilter:"required,field=password,doc=The new password to set."`
}

type SetPasswordFunction struct{}

func (self SetPasswordFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &SetPasswordFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("passwd: %v", err)
		return vfilter.Null{}
	}

	if len(arg.Password) < 4 {
		scope.Log("passwd: Password is not set or too short")
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("password: %v", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("passwd: Command can only run on the server")
		return vfilter.Null{}
	}

	users_manager := services.GetUserManager()
	err = users_manager.SetUserPassword(ctx, config_obj, principal, arg.Username,
		arg.Password, "")
	if err != nil {
		scope.Log("passwd: %v", err)
		return vfilter.Null{}
	}

	return arg.Username
}

func (self SetPasswordFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "passwd",
		Doc:     "Updates the user's password.",
		ArgType: type_map.AddType(scope, &SetPasswordFunctionArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&SetPasswordFunction{})
}
