/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2022 Rapid7 Inc.

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
	"errors"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	global_user_manager UserManager

	UserNotFoundError = errors.New("User not found")
)

type UserManager interface {
	SetUser(ctx context.Context,
		user_record *api_proto.VelociraptorUser) error
	GetUser(ctx context.Context,
		username string) (*api_proto.VelociraptorUser, error)

	ListUsers(ctx context.Context) ([]*api_proto.VelociraptorUser, error)
	GetUserFromContext(ctx context.Context) (
		*api_proto.VelociraptorUser, *config_proto.Config, error)

	GetUserWithHashes(ctx context.Context, username string) (*api_proto.VelociraptorUser, error)
	SetUserOptions(ctx context.Context, username string,
		options *api_proto.SetGUIOptionsRequest) error
	GetUserOptions(ctx context.Context, username string) (*api_proto.SetGUIOptionsRequest, error)

	// Favorites are stored per org.
	GetFavorites(ctx context.Context, config_obj *config_proto.Config,
		principal, fav_type string) (*api_proto.Favorites, error)

	DeleteUser(ctx context.Context, config_obj *config_proto.Config, username string) error
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
