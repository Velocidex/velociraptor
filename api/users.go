package api

import (
	"errors"
	"os"
	"sort"

	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/users"
)

// This is only used to set the user's own password which is always
// allowed for any user.
func (self *ApiServer) SetPassword(
	ctx context.Context,
	in *api_proto.SetPasswordRequest) (*emptypb.Empty, error) {

	// Enforce a minimum length password
	if len(in.Password) < 4 {
		return nil, InvalidStatus("Password is not set or too short")
	}

	user_manager := services.GetUserManager()
	user_record, _, err := user_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	principal := user_record.Name
	err = users.SetUserPassword(ctx, principal, principal, in.Password, "")
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	org_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	logger := logging.GetLogger(org_config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  user_record.Name,
		"Principal": user_record.Name,
	}).Info("passwd: Updating password for user via API")

	return &emptypb.Empty{}, nil
}

func (self *ApiServer) GetUsers(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.Users, error) {

	user_manager := services.GetUserManager()
	user_record, org_config_obj, err := user_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	principal := user_record.Name

	// Only show users in the current org
	users, err := users.ListUsers(ctx, principal, []string{org_config_obj.OrgId})
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	return &api_proto.Users{Users: users}, nil
}

func (self *ApiServer) GetGlobalUsers(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.Users, error) {

	user_manager := services.GetUserManager()
	user_record, _, err := user_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	principal := user_record.Name

	// Show all users visible to us
	users, err := users.ListUsers(ctx, principal, []string{})
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	return &api_proto.Users{Users: users}, nil
}

func (self *ApiServer) ChangeUser(ctx context.Context,
				  in *api_proto.UpdateUserRequest,
				  options users.AddUserOptions) (*emptypb.Empty, error) {

	users_manager := services.GetUserManager()
	principal, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	per_org_policy := map[string]*acl_proto.ApiClientACL{}
	for org, roles := range in.RolesPerOrg {
		per_org_policy[org] = &acl_proto.ApiClientACL{
			Roles: roles.Strings,
		}
	}

	err = users.AddUserToOrg(ctx, options, principal.Name, in.Name, per_org_policy)
	if err != nil {
//		if errors.Is(err, users.ErrInvalidArgument) {
//			return nil, status.Error(codes.InvalidArgument, err.Error())
//		} else if errors.Is(err, users.ErrPermissionDenied) {
//			return nil, status.Error(codes.PermissionDenied,
//					"User is not allowed to create users.")
//		} else if errors.Is(err, users.ErrUserNotFound) {
//			return nil, status.Errorf(codes.NotFound, "User %s does not exist.",
//						  in.Name)
//		} else if errors.Is(err, users.ErrUserAlreadyExists) {
//			return nil, status.Errorf(codes.AlreadyExists,
//						  "Cannot create user %s.  Username already exists",
//						  in.Name)
//		}
		return nil, err
	}

	if in.Password != "" {
		logger := logging.GetLogger(org_config_obj, &logging.APICmponent)
		logger.WithFields(logrus.Fields{
			"Principal":  principal.Name,
			"Username": in.Name,
		}).Info("users: setting password")
		err = users.SetUserPassword(ctx, principal.Name, in.Name, in.Password, "")
		if err != nil {
			return nil, err
		}
	}

	msg := "users: Created user record via API"
	if options == users.UseExistingUser {
		msg = "users: Updated user record via API"
	}

	logger := logging.GetLogger(org_config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Principal":  principal.Name,
		"Username": in.Name,
	}).Info(msg)

	return &emptypb.Empty{}, nil
}

func (self *ApiServer) CreateUser(ctx context.Context,
				  in *api_proto.UpdateUserRequest) (*emptypb.Empty, error) {
	return self.ChangeUser(ctx, in, users.AddNewUser)
}

func (self *ApiServer) UpdateUser(ctx context.Context,
				  in *api_proto.UpdateUserRequest) (*emptypb.Empty, error) {
	return self.ChangeUser(ctx, in, users.UseExistingUser)
}

func (self *ApiServer) DeleteUser(ctx context.Context,
				  in *api_proto.DeleteUserRequest) (*emptypb.Empty, error) {

	users_manager := services.GetUserManager()
	principal, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	orgs := in.Orgs
	if len(orgs) == 0 {
		orgs = users.LIST_ALL_ORGS
	}

	err = users.DeleteUser(ctx, principal.Name, in.Name, orgs)
	if err != nil {
		return nil, err
	}

	logger := logging.GetLogger(org_config_obj, &logging.Audit)
	logger.WithFields(logrus.Fields{
		"Username":  in.Name,
		"Principal": principal,
	}).Info("users: Deleted user record via API")

	return &emptypb.Empty{}, nil
}

func (self *ApiServer) GetUser(
	ctx context.Context, in *api_proto.UserRequest) (*api_proto.VelociraptorUser, error) {

	users_manager := services.GetUserManager()
	user_record, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user, err := users.GetUser(ctx, user_record.Name, in.Name)
	if err != nil {
		if errors.Is(err, acls.PermissionDenied) {
			return nil, status.Error(codes.PermissionDenied,
                                       "User is not allowed to view requested user.")
	       }
		return nil, err
	}

	return user, nil
}

func (self *ApiServer) GetUserFavorites(
	ctx context.Context,
	in *api_proto.Favorite) (*api_proto.Favorites, error) {

	// No special permission requires to view a user's own favorites.
	users_manager := services.GetUserManager()
	user_record, org_config_obj, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name
	return users_manager.GetFavorites(ctx, org_config_obj, principal, in.Type)
}

func (self *ApiServer) GetUserRoles(
	ctx context.Context,
	in *api_proto.UserRequest) (*api_proto.UserRoles, error) {

	users_manager := services.GetUserManager()
	user_record, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user, err := users.GetUser(ctx, user_record.Name, in.Name)
	if err != nil {
		if errors.Is(err, acls.PermissionDenied) {
			return nil, status.Error(codes.PermissionDenied,
                                       "User is not allowed to view requested user.")
	       }
		return nil, err
	}


	user_roles := &api_proto.UserRoles{
		Name: in.Name,
		RolesPerOrg: map[string]*api_proto.Strings{},
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	for _, user_org := range user.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
		if err != nil {
			return nil, Status(self.verbose, err)
		}
		acl, err := services.GetPolicy(org_config_obj, in.Name)
		if err != nil {
			// No ACL in this org isn't an error
			if !errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, Status(self.verbose, err)
		}
		user_roles.RolesPerOrg[user_org.Id] = &api_proto.Strings{Strings: acl.Roles}
	}
	return user_roles, nil
}

func (self *ApiServer) SetUserRoles(
	ctx context.Context,
	in *api_proto.UserRoles) (*emptypb.Empty, error) {

	users_manager := services.GetUserManager()
	user_record, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	principal := user_record.Name

	per_org_policy := map[string]*acl_proto.ApiClientACL{}

	for org, roles := range in.RolesPerOrg {
		per_org_policy[org] = &acl_proto.ApiClientACL{Roles: roles.Strings,}
	}

	err = users.GrantUserToOrg(ctx, principal, in.Name, per_org_policy)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &emptypb.Empty{}, nil
}
