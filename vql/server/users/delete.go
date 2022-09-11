package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserDeleteFunctionArgs struct {
	Username string `vfilter:"required,field=user,doc=The user to delete."`
}

type UserDeleteFunction struct{}

func (self UserDeleteFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &UserDeleteFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	user_manager := services.GetUserManager()

	principal := vql_subsystem.GetPrincipal(scope)
	logger := logging.GetLogger(config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  arg.Username,
		"Principal": principal,
	}).Info("user_delete")

	err = user_manager.DeleteUser(ctx, config_obj, arg.Username)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
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
