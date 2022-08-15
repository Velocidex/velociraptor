package users

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
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

	principal := vql_subsystem.GetPrincipal(scope)
	if arg.Username != "" && arg.Username != principal {
		// Only an admin can set another user's password.
		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log(
				"passwd: %v setting %v password:  %v",
				principal, arg.Username, err)
			return vfilter.Null{}
		}

		// Allow an admin to set another user's password.
		principal = arg.Username
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("passwd: Command can only run on the server")
		return vfilter.Null{}
	}

	authenticator, err := authenticators.NewAuthenticator(config_obj)
	if err != nil {
		scope.Log("passwd: %v", err)
		return vfilter.Null{}
	}

	if authenticator.IsPasswordLess() {
		scope.Log("passwd: Authenticator is passwordless.")
		return vfilter.Null{}
	}

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUserWithHashes(principal)
	if err == services.UserNotFoundError {
		scope.Log("passwd: %v", err)
		return vfilter.Null{}
	}
	if err != nil {
		scope.Log("passwd: %v", err)
		return vfilter.Null{}
	}

	// Set the password on the record.
	users.SetPassword(user_record, arg.Password)

	logger := logging.GetLogger(config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  arg.Username,
		"Principal": principal,
	}).Info("passwd: Updating password for user")

	// Store the record
	err = users_manager.SetUser(user_record)
	if err != nil {
		scope.Log("passwd: Unable to set user account: %v", err)
		return vfilter.Null{}
	}

	return principal
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
