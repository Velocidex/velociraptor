/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package services

import (
	"context"

	acl_proto "www.velocidex.com/golang/velociraptor/acls/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	global_user_manager UserManager

	UserNotFoundError = utils.Wrap(utils.NotFoundError, "User not found")

	LIST_ALL_ORGS []string = nil
)

type AddUserOptions int

const (
	UseExistingUser AddUserOptions = iota
	AddNewUser
)

/*
The user manager is global to all orgs and therefore it is
initialized once for the root org.

This is the one stop shop for managing everything about users except
ACLs (which are managed within in org seperately)
*/
type UserManager interface {
	SetUser(ctx context.Context,
		user_record *api_proto.VelociraptorUser) error

	GetUser(ctx context.Context, principal, username string) (
		*api_proto.VelociraptorUser, error)

	// Verify the User password (only used for Basic Authentication.
	VerifyPassword(
		ctx context.Context,
		principal, username string,
		password string) (bool, error)

	// List all users in these orgs.
	ListUsers(ctx context.Context,
		principal string, orgs []string) ([]*api_proto.VelociraptorUser, error)

	GetUserFromContext(ctx context.Context) (
		*api_proto.VelociraptorUser, *config_proto.Config, error)

	GetUserFromHTTPContext(ctx context.Context) (
		*api_proto.VelociraptorUser, error)

	// Used to get the user's record including password hashes. This
	// only makes sense when using the `Basic` authenticator because
	// otherwise we dont maintain passwords.
	GetUserWithHashes(ctx context.Context, principal, username string) (
		*api_proto.VelociraptorUser, error)

	// Used to set and retrieve user GUI options.
	SetUserOptions(ctx context.Context, principal, username string,
		options *api_proto.SetGUIOptionsRequest) error

	GetUserOptions(ctx context.Context, username string) (
		*api_proto.SetGUIOptionsRequest, error)

	// Favorites are stored per org because they refer to artifacts
	// which may be specific for each org.
	GetFavorites(ctx context.Context, config_obj *config_proto.Config,
		principal, fav_type string) (*api_proto.Favorites, error)

	// List all the orgs the user can see (i.e the users has READER
	// level access)
	GetOrgs(ctx context.Context, principal string) []*api_proto.OrgRecord

	// Adds the user to the org.
	// - OrgAdmin can add any user to any org.
	// - ServerAdmin is required in all orgs.

	// If the user account does not exist and the AddNewUser option is
	// provided, we create a new user account.

	// This function effectively grants permissions in the org so it is
	// the same as GrantUserInOrg
	AddUserToOrg(ctx context.Context,
		options AddUserOptions,
		principal, username string,
		orgs []string, policy *acl_proto.ApiClientACL) error

	// Update the user's password.
	// A user may update their own password.
	// A ServerAdmin in any of the orgs the user belongs to can update their password.
	// An OrgAdmin can update everyone's password
	SetUserPassword(
		ctx context.Context,
		org_config_obj *config_proto.Config,
		principal, username string,
		password, current_org string) error

	// Removes the user record.
	// principal - is the user who is requesting this account removal.
	// username - the user to remove.
	// orgs - The list of orgs to remove the user from.
	DeleteUser(
		ctx context.Context,
		principal, username string,
		orgs []string) error
}

// A helper
func GrantUserToOrg(
	ctx context.Context,
	principal, username string,
	orgs []string, policy *acl_proto.ApiClientACL) error {

	users_manager := GetUserManager()
	return users_manager.AddUserToOrg(ctx, UseExistingUser,
		principal, username, orgs, policy)
}

func RegisterUserManager(dispatcher UserManager) {
	mu.Lock()
	defer mu.Unlock()

	global_user_manager = dispatcher
}

func GetUserManager() UserManager {
	mu.Lock()
	defer mu.Unlock()

	return global_user_manager
}
