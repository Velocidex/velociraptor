package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

type AddUserOptions int

const (
	UseExistingUser AddUserOptions = iota
	AddNewUser
)

// Adds the user to the org.
// - OrgAdmin can add any user to any org.
// - ServerAdmin is required in all orgs.

// If the user account does not exist and the AddNewUser option is
// provided, we create a new user account.

// This function effectively grants permissions in the org so it is
// the same as GrantUserInOrg
func AddUserToOrg(
	ctx context.Context,
	options AddUserOptions,
	principal, username string,
	orgs []string, policy *acl_proto.ApiClientACL) error {

	if isNameReserved(username) {
		return NameReservedError
	}

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return err
	}

	ok, _ := services.CheckAccess(root_config_obj, principal, acls.ORG_ADMIN)
	if !ok {
		// Check that all the orgs have ServerAdmin
		for _, org := range orgs {
			org_config_obj, err := org_manager.GetOrgConfig(org)
			if err != nil {
				return err
			}

			ok, _ := services.CheckAccess(
				org_config_obj, principal, acls.SERVER_ADMIN)
			if !ok {
				return acls.PermissionDenied
			}
		}
	}

	user_manager := services.GetUserManager()

	// Hold on to the error until after ACL check
	user_record, err := user_manager.GetUserWithHashes(ctx, username)
	if err != nil {
		if err == services.UserNotFoundError &&
			options == UseExistingUser {
			return err
		}

		// Create a new user object. Password will need to be set
		// seperately through SetUserPassword()
		user_record = &api_proto.VelociraptorUser{
			Name: username,
		}
	}

	for _, org := range orgs {
		if !inUserOrgs(org, user_record) {
			user_record.Orgs = append(user_record.Orgs, &api_proto.Org{
				Id: org,
			})
		}

		org_config_obj, err := org_manager.GetOrgConfig(org)
		if err != nil {
			return err
		}

		// Grant the user the ACL in the specified Orgs.
		err = services.SetPolicy(org_config_obj, username, policy)
		if err != nil {
			return err
		}

	}

	return user_manager.SetUser(ctx, user_record)
}

func GrantUserToOrg(
	ctx context.Context,
	principal, username string,
	orgs []string, policy *acl_proto.ApiClientACL) error {
	return AddUserToOrg(ctx, UseExistingUser,
		principal, username, orgs, policy)
}

// We dont expect too many orgs so O(1) is ok.
func inUserOrgs(org_id string, user_record *api_proto.VelociraptorUser) bool {
	for _, org := range user_record.Orgs {
		if org_id == org.Id {
			return true
		}
	}
	return false
}
