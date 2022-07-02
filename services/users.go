/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

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

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	global_user_manager UserManager
)

type UserManager interface {
	SetUser(user_record *api_proto.VelociraptorUser) error
	ListUsers() ([]*api_proto.VelociraptorUser, error)
	GetUserFromContext(ctx context.Context) (
		*api_proto.VelociraptorUser, *config_proto.Config, error)
	GetUser(username string) (*api_proto.VelociraptorUser, error)
	GetUserWithHashes(username string) (*api_proto.VelociraptorUser, error)
	SetUserOptions(username string,
		options *api_proto.SetGUIOptionsRequest) error
	GetUserOptions(username string) (*api_proto.SetGUIOptionsRequest, error)

	// Favorites are stored per org.
	GetFavorites(config_obj *config_proto.Config,
		principal, fav_type string) (*api_proto.Favorites, error)
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
