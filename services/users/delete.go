package users

import (
	"context"

	"github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Removes a user from an org
func (self *UserManager) DeleteUser(
	ctx context.Context,
	principal, username string,
	orgs []string) error {

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

	principal_is_org_admin, err := services.CheckAccess(
		root_config_obj, principal, acls.ORG_ADMIN)
	if err != nil {
		return err
	}

	// Hold on to the error until after ACL check.  Get the full
	// unfiltered user record with all the orgs they belong to so we
	// can remove those orgs the principal is allowed to touch and put
	// the rest back.
	user_record, user_err := self.storage.GetUserWithHashes(ctx, username)
	if user_err != nil {
		if principal_is_org_admin {
			return user_err
		}
		return errors.Errorf("Error %v: User %v is not org admin",
			acls.PermissionDenied, principal)
	}

	// Fill in the orgs this user is in.
	self.normalizeOrgList(ctx, user_record)

	// Empty policy - no permissions.
	policy := &acl_proto.ApiClientACL{}

	orgs_deleted := 0
	for _, user_org := range user_record.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
		if err != nil {
			continue
		}

		// Skip orgs that are not specified.
		if len(orgs) > 0 && !utils.OrgIdInList(user_org.Id, orgs) {
			continue
		}

		// Further checks if the principal is not ORG_ADMIN
		if !principal_is_org_admin {
			ok, _ := services.CheckAccess(
				org_config_obj, principal, acls.SERVER_ADMIN)
			if !ok {
				// If the user is not server admin on this org they
				// may not remove the user from this org
				continue
			}
		}

		// Reset the user's ACLs in this org.
		err = services.SetPolicy(org_config_obj, username, policy)
		if err != nil {
			return err
		}
		orgs_deleted++
	}

	// If no more orgs remain, delete the actual user record.
	if orgs_deleted >= len(user_record.Orgs) {
		return self.storage.DeleteUser(ctx, username)
	}

	return nil
}
