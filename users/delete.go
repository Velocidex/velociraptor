package users

import (
	"context"

	"www.velocidex.com/golang/velociraptor/acls"
	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

// Removes a user from an org
func DeleteUser(
	ctx context.Context,
	principal, username string,
	orgs []string) error {

	if isNameReserved(username) {
		return NameReservedError
	}

	user_manager := services.GetUserManager()

	// Hold on to the error until after ACL check
	user_record, user_err := user_manager.GetUserWithHashes(ctx, username)

	org_manager, err := services.GetOrgManager()
	if err != nil {
		return err
	}

	root_config_obj, err := org_manager.GetOrgConfig(services.ROOT_ORG_ID)
	if err != nil {
		return err
	}

	principal_is_org_admin, _ := services.CheckAccess(
		root_config_obj, principal, acls.ORG_ADMIN)

	if user_err != nil {
		if principal_is_org_admin {
			return user_err
		}
		return acls.PermissionDenied
	}

	remaining_orgs := []*api_proto.Org{}
	// Empty policy - no permissions.
	policy := &acl_proto.ApiClientACL{}

	for _, user_org := range user_record.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
		if err != nil {
			remaining_orgs = append(remaining_orgs, user_org)
			continue
		}

		// Skip orgs that are not specified.
		if len(orgs) > 0 && !utils.OrgIdInList(user_org.Id, orgs) {
			remaining_orgs = append(remaining_orgs, user_org)
			continue
		}

		// Further checks if the principal is not ORG_ADMIN
		if !principal_is_org_admin {
			ok, _ := services.CheckAccess(
				org_config_obj, principal, acls.SERVER_ADMIN)
			if !ok {
				// If the user is not server admin on this org they
				// may not remove the user from this org
				remaining_orgs = append(remaining_orgs, user_org)
				continue
			}
		}

		// Reset the user's ACLs in this org.
		err = services.SetPolicy(org_config_obj, username, policy)
		if err != nil {
			return err
		}
	}

	if len(remaining_orgs) > 0 {
		// Update the user's record
		user_record.Orgs = remaining_orgs
		return user_manager.SetUser(ctx, user_record)
	}

	// No more orgs for this user, Just remove the user completely
	return user_manager.DeleteUser(ctx, root_config_obj, username)
}
