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
package api

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

func NewDefaultUserObject(config_obj *config_proto.Config) *api_proto.ApiUser {
	result := &api_proto.ApiUser{
		UserType:        api_proto.ApiUser_USER_TYPE_ADMIN,
		InterfaceTraits: &api_proto.ApiUserInterfaceTraits{},
	}

	if config_obj.GUI != nil {
		result.InterfaceTraits = &api_proto.ApiUserInterfaceTraits{
			Links: config_obj.GUI.Links,
		}
	}

	return result
}
