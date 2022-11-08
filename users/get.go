package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Gets the user record.
// If the principal == username the user is getting their own record.
// If the principal is a SERVER_ADMIN in any orgs the user belongs in
// they can see the full record.
// If the principal has READER in any orgs the user belongs in they
// can only see the user name.
func GetUser(
	ctx context.Context,
	principal, username string) (*api_proto.VelociraptorUser, error) {

	result, err := getUserWithHashes(ctx, principal, username)
	if err != nil {
		return nil, err
	}

	result.PasswordHash = nil
	result.PasswordSalt = nil

	return result, nil
}

func getUserWithHashes(
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

	user_manager := services.GetUserManager()

	// Hold on to the error until after ACL check
	user, user_err := user_manager.GetUserWithHashes(ctx, username)

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

	allowed_full := false
	returned_orgs := []*api_proto.Org{}

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

	if allowed_full {
		user_record.Orgs = returned_orgs
	}
	return user_record, nil
}
