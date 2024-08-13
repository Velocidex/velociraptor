package users

import (
	"context"
	"errors"
	"fmt"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Adds the user to the org.
// - OrgAdmin can add any user to any org.
// - ServerAdmin is required in all orgs.

// If the user account does not exist and the AddNewUser option is
// provided, we create a new user account.

// This function effectively grants permissions in the org so it is
// the same as GrantUserInOrg
func (self *UserManager) AddUserToOrg(
	ctx context.Context,
	options services.AddUserOptions,
	principal, username string,
	orgs []string, policy *acl_proto.ApiClientACL) error {

	err := ValidateUsername(self.config_obj, username)
	if err != nil {
		return err
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
				return fmt.Errorf("Error: %w, User %v is not admin on %v",
					acls.PermissionDenied, principal, org_config_obj.OrgName)
			}
		}
	}

	// Hold on to the error until after ACL check.  Get the full
	// unfiltered user record with all the orgs they belong to so we
	// can remove those orgs the principal is allowed to touch and put
	// the rest back.
	user_record, err := self.storage.GetUserWithHashes(ctx, username)
	if err != nil {
		if errors.Is(err, services.UserNotFoundError) &&
			options == services.UseExistingUser {
			return err
		}

		// Create a new user object. Password will need to be set
		// seperately through SetUserPassword()
		user_record = &api_proto.VelociraptorUser{
			Name: username,
		}
	}

	for _, org := range orgs {
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

	return self.SetUser(ctx, user_record)
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
