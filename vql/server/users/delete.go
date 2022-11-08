package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/users"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserDeleteFunctionArgs struct {
	Username string   `vfilter:"required,field=user,doc=The user to delete."`
	OrgIds   []string `vfilter:"optional,field=orgs,doc=If set we only delete from these orgs, otherwise all the orgs the principal has SERVER_ADMIN on."`
}

type UserDeleteFunction struct{}

func (self UserDeleteFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	// ACLs are checked by the users module
	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("user_delete, Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &UserDeleteFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	orgs := users.LIST_ALL_ORGS
	if len(arg.OrgIds) == 0 {
		orgs = []string{config_obj.OrgId}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	logger := logging.GetLogger(config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  arg.Username,
		"Principal": principal,
	}).Info("user_delete")

	err = users.DeleteUser(ctx, principal, arg.Username, orgs)
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
