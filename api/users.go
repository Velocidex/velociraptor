package api

import (
	"errors"
	"sort"

	context "golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	org_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// The user we change the password for.
	target := in.Username
	if target == "" {
		target = principal
	}

	err = users.SetUserPassword(
		ctx, org_config_obj, principal, target, in.Password, "")
	if err != nil {
		return nil, Status(self.verbose, err)
	}

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

// Create a new user in the specified orgs.
func (self *ApiServer) CreateUser(ctx context.Context,
	in *api_proto.UpdateUserRequest) (*emptypb.Empty, error) {

	users_manager := services.GetUserManager()
	user_record, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	principal := user_record.Name

	// Prepare an ACL object from the incoming request.
	acl := &acl_proto.ApiClientACL{
		Roles: in.Roles,
	}

	mode := users.UseExistingUser
	if in.AddNewUser {
		mode = users.AddNewUser
	}

	err = users.AddUserToOrg(ctx, mode, principal, in.Name, in.Orgs, acl)
	return &emptypb.Empty{}, err
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
	_, _, err := users_manager.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Allow the user to ask about other orgs.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	org_config_obj, err := org_manager.GetOrgConfig(in.Org)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	acl_manager, err := services.GetACLManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	policy, err := acl_manager.GetPolicy(org_config_obj, in.Name)
	if err != nil {
		policy = &acl_proto.ApiClientACL{}
	}

	user_roles := &api_proto.UserRoles{
		Name:           in.Name,
		Org:            in.Org,
		OrgName:        org_config_obj.OrgName,
		Roles:          policy.Roles,
		Permissions:    acls.DescribePermissions(policy),
		AllRoles:       acls.ALL_ROLES,
		AllPermissions: acls.ALL_PERMISSIONS,
	}

	// Expand the policy's permissions
	err = acls.GetRolePermissions(org_config_obj, policy.Roles, policy)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user_roles.EffectivePermissions = acls.DescribePermissions(policy)

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

	// Prepare an ACL object from the incoming request.
	acl := &acl_proto.ApiClientACL{}

	for _, r := range in.Roles {
		if acls.ValidateRole(r) {
			acl.Roles = append(acl.Roles, r)
		}
	}

	// Add any special permissions
	err = acls.SetTokenPermission(acl, in.Permissions...)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Now attempt to set the ACL - permission checks are done by
	// users.AddUserToOrg
	err = users.AddUserToOrg(ctx, users.UseExistingUser,
		principal, in.Name, []string{in.Org}, acl)
	return &emptypb.Empty{}, err
}
