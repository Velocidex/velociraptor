package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
	NameReservedError = errors.New("Username is reserved")
)

// Update the user's password.
// A user may update their own password.
// A ServerAdmin in any of the orgs the user belongs to can update their password.
// An OrgAdmin can update everyone's password
func SetUserPassword(
	ctx context.Context,
	config_obj *config_proto.Config,
	principal, username string,
	password, current_org string) error {

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

	user_manager := services.GetUserManager()

	// Hold on to the error until after ACL check
	user_record, user_err := user_manager.GetUser(ctx, username)

	// Update the password if needed.
	if password != "" {
		setPassword(user_record, password)
	}

	// Update the current org if needed
	if current_org != "" {
		// Check if the current_org is in the list of user orgs
		if !inUserOrgs(current_org, user_record) {
			return fmt.Errorf("Error %v: Org %v does not include User %v",
				acls.PermissionDenied, current_org, user_record.Name)
		}
		user_record.CurrentOrg = current_org
	}

	// A user can always get their own user record regarless of
	// permissions.
	if principal == username {
		if user_err != nil {
			return user_err
		}
		services.LogAudit(ctx,
			config_obj, principal, "Update password",
			ordereddict.NewDict().
				Set("operation", "Update Own Password").
				Set("user", user_record.Name))

		return user_manager.SetUser(ctx, user_record)
	}

	// ORG_ADMINs can see everything
	ok, _ := services.CheckAccess(root_config_obj, principal, acls.ORG_ADMIN)
	if ok {
		if user_err != nil {
			return user_err
		}

		services.LogAudit(ctx,
			config_obj, principal, "Update password",
			ordereddict.NewDict().
				Set("operation", "Update Password By Admin").
				Set("user", user_record.Name))

		return user_manager.SetUser(ctx, user_record)
	}

	for _, user_org := range user_record.Orgs {
		org_config_obj, err := org_manager.GetOrgConfig(user_org.Id)
		if err != nil {
			continue
		}

		ok, _ := services.CheckAccess(
			org_config_obj, principal, acls.SERVER_ADMIN)
		if ok {
			if user_err != nil {
				return user_err
			}
			services.LogAudit(ctx,
				config_obj, principal, "Update password",
				ordereddict.NewDict().
					Set("operation", "Update Password By Admin").
					Set("user", user_record.Name))

			return user_manager.SetUser(ctx, user_record)
		}
	}

	services.LogAudit(ctx,
		config_obj, principal, "Update password",
		ordereddict.NewDict().
			Set("error", acls.PermissionDenied.Error()).
			Set("user", user_record.Name))

	return acls.PermissionDenied
}

func setPassword(user_record *api_proto.VelociraptorUser, password string) {
	salt := make([]byte, 32)
	_, err := rand.Read(salt)
	if err != nil {
		return
	}
	hash := sha256.Sum256(append(salt, []byte(password)...))
	user_record.PasswordSalt = salt[:]
	user_record.PasswordHash = hash[:]
	user_record.Locked = false
}

func verifyPassword(self *api_proto.VelociraptorUser, password string) bool {
	hash := sha256.Sum256(append(self.PasswordSalt, []byte(password)...))
	return subtle.ConstantTimeCompare(hash[:], self.PasswordHash) == 1
}

func VerifyPassword(
	ctx context.Context,
	principal, username string,
	password string) (bool, error) {

	user_record, err := getUserWithHashes(ctx, principal, username)
	if err != nil {
		return false, err
	}

	return verifyPassword(user_record, password), nil
}

// Store this special name from being added - This principal is used
// internally by the server to bypass the ACL system when needed.
func isNameReserved(username string) bool {
	switch username {
	case constants.PinnedServerName:
		return true
	}
	return false
}
