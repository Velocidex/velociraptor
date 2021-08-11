package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/paths"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserDeleteFunctionArgs struct {
	Username string `vfilter:"required,field=user,docs=The user to delete."`
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

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	user_path_manager := paths.NewUserPathManager(arg.Username)
	err = db.DeleteSubject(config_obj, user_path_manager.Path())
	if err != nil {
		scope.Log("user_delete: %s", err)
		return vfilter.Null{}
	}

	// Also remove the ACLs for the user.
	err = db.DeleteSubject(config_obj, user_path_manager.ACL())
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
