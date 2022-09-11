package users

import (
	"context"
	"crypto/rand"

	"github.com/Velocidex/ordereddict"
	"github.com/sirupsen/logrus"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/users"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type UserCreateFunctionArgs struct {
	Username string   `vfilter:"required,field=user,doc=The user to create or update."`
	Roles    []string `vfilter:"required,field=roles,doc=List of roles to give the user."`
	Password string   `vfilter:"optional,field=password,doc=A password to set for the user (If not using SSO this might be needed)."`
	OrgIds   []string `vfilter:"optional,field=orgs,doc=One or more org IDs to grant access to."`
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

	arg := &UserCreateFunctionArgs{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("Command can only run on the server")
		return vfilter.Null{}
	}

	users_manager := services.GetUserManager()
	user_record, err := users_manager.GetUserWithHashes(ctx, arg.Username)
	if err == services.UserNotFoundError {
		// OK - Lets make the user now
		user_record, err = users.NewUserRecord(arg.Username)
		if err != nil {
			scope.Log("user_create: %s", err)
			return vfilter.Null{}
		}

	} else if err != nil {
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

	} else if len(user_record.PasswordHash) > 0 {
		// Password is already set - leave it alone.

	} else if arg.Password == "" {
		// Do not accept an empty password if we are using a password
		// based authenticator.
		scope.Log("Authentication requires a password but one was not provided.")
		return vfilter.Null{}

	} else {
		users.SetPassword(user_record, arg.Password)
	}

	// Grat the user the roles in all orgs.
	org_config_obj := config_obj

	// No OrgIds specified - the user will be created in the root org.
	if len(arg.OrgIds) == 0 {
		// Grant the roles to the user
		err = acls.GrantRoles(config_obj, arg.Username, arg.Roles)
		if err != nil {
			scope.Log("user_create: %s", err)
			return vfilter.Null{}
		}

		// OrgIds specified, grant the user an ACL in each org specified.
	} else {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			scope.Log("user_create: %v", err)
			return vfilter.Null{}
		}

		for _, org_id := range arg.OrgIds {
			org_config_obj, err = org_manager.GetOrgConfig(org_id)
			if err != nil {
				scope.Log("user_create: %v", err)
				return vfilter.Null{}
			}

			// Grant the roles to the user
			err = acls.GrantRoles(org_config_obj, arg.Username, arg.Roles)
			if err != nil {
				scope.Log("user_create: %s", err)
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

	// Write the user record.
	err = users_manager.SetUser(ctx, user_record)
	if err != nil {
		scope.Log("user_create: %s", err)
		return vfilter.Null{}
	}

	principal := vql_subsystem.GetPrincipal(scope)
	logger := logging.GetLogger(config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  arg.Username,
		"Roles":     arg.Roles,
		"OrgIds":    arg.OrgIds,
		"Principal": principal,
	}).Info("user_create")

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
