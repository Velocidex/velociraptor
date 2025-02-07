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
	"context"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

func GetUserInfo(ctx context.Context,
	config_obj *config_proto.Config) *api_proto.VelociraptorUser {
	result := &api_proto.VelociraptorUser{}

	userinfo, ok := ctx.Value(constants.GRPC_USER_CONTEXT).(string)
	if ok {
		data := []byte(userinfo)
		err := json.Unmarshal(data, result)
		if err != nil {
			logging.GetLogger(config_obj, &logging.GUIComponent).Error(
				"Unable to Unmarshal USER Token")
		}
	}
	return result
}
