package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Returns the user record after stripping sensitive information like
// password hashes.

// Gets the user record.
// If the principal == username the user is getting their own record.
// If the principal is a SERVER_ADMIN in any orgs the user belongs in
// they can see the full record.
// If the principal has READER in any orgs the user belongs in they
// can only see the user name.
func (self *UserManager) GetUser(
	ctx context.Context, principal, username string) (*api_proto.VelociraptorUser, error) {

	// For the server name we dont have a real user record, we make a
	// hard coded user record instead.
	if username == utils.GetSuperuserName(self.config_obj) {
		return &api_proto.VelociraptorUser{
			Name: username,
		}, nil
	}

	// Call our overloaded method which check permissions and
	// visibility.
	result, err := self.GetUserWithHashes(ctx, principal, username)
	if err != nil {
		return nil, err
	}

	// Clear the hashes
	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

func (self *UserManager) GetUserWithHashes(
	ctx context.Context,
	principal, username string) (*api_proto.VelociraptorUser, error) {

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return nil, err
	}

	// Hold on to the error until after ACL check
	user, user_err := self.storage.GetUserWithHashes(ctx, username)
	if user_err != nil {
		return nil, user_err
	}

	// Fill in the org memberships for the user record using the org
	// manager.
	self.normalizeOrgList(ctx, user)

	// A user can always get their own user record regarless of
	// permissions.
	if principal == username {
		return user, user_err
	}

	// ORG_ADMINs can see everything
	ok, _ := services.CheckAccess(root_config_obj, principal, acls.ORG_ADMIN)
	if ok {
		return user, user_err
	}

	if user == nil {
		return nil, acls.PermissionDenied
	}

	// Filter the org list according to the principal's permissions
	// and visibility rules.
	allowed_full := false
	returned_orgs := []*api_proto.OrgRecord{}

	for _, user_org := range user.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
		if err != nil {
			continue
		}

		ok, _ := services.CheckAccess(
			org_config_obj, principal, acls.SERVER_ADMIN)
		if ok {
			returned_orgs = append(returned_orgs, user_org)
			allowed_full = true
			continue
		}

		ok, _ = services.CheckAccess(org_config_obj,
			principal, acls.READ_RESULTS)
		if ok {
			if user_err == nil {
				return &api_proto.VelociraptorUser{
					Name: username,
				}, nil
			}

			returned_orgs = append(returned_orgs, user_org)
			continue
		}
	}

	// None of the orgs the user belongs to give the principal
	// permission to get this user record.
	if len(returned_orgs) == 0 {
		return nil, acls.PermissionDenied
	}

	// This is the record we will return.
	user_record := &api_proto.VelociraptorUser{
		Name: user.Name,
	}

	// We are allowed to return a filtered list of org memberships.
	if allowed_full {
		user_record.Orgs = returned_orgs
	}
	return user_record, nil
}
