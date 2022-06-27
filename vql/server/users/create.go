package users

import (
	"context"
	"crypto/rand"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserCreateFunctionArgs struct {
	Username string   `vfilter:"required,field=user,docs=The user to create or update."`
	Roles    []string `vfilter:"required,field=roles,docs=List of roles to give the user."`
	Password string   `vfilter:"optional,field=password,docs=A password to set for the user (If not using SSO this might be needed)."`
	OrgIds   []string `vfilter:"optional,field=orgs,docs=One or more org IDs to grant access to."`
}

type UserCreateFunction struct{}

func (self UserCreateFunction) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	arg := &UserCreateFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	// OK - Lets make the user now
	user_record, err := users.NewUserRecord(arg.Username)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	// Check the password if needed
	authenticator, err := authenticators.NewAuthenticator(config_obj)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	if authenticator.IsPasswordLess() {
		// Set a random password on the account to prevent login if
		// the authenticator is accidentally changed to a password
		// based one.
		password := make([]byte, 100)
		_, err = rand.Read(password)
		if err != nil {
			scope.Log("user_create: %s", err)
			return vfilter.Null{}
		}
		users.SetPassword(user_record, string(password))

	} else if arg.Password == "" {
		// Do not accept an empty password if we are using a password
		// based authenticator.
		scope.Log("Authentication requires a password but one was not provided.")
		return vfilter.Null{}

	} else {
		users.SetPassword(user_record, arg.Password)
	}

	// Grant the roles to the user
	err = acls.GrantRoles(config_obj, arg.Username, arg.Roles)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	// Write the user record.
	users_manager := services.GetUserManager()
	err = users_manager.SetUser(config_obj, user_record)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
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
