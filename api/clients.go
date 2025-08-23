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
	"errors"
	"os"
	"time"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) GetClientMetadata(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ClientMetadata, error) {

	defer Instrument("GetClientMetadata")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	if in.ClientId == "server" {
		permissions = acls.SERVER_ADMIN
	}

	perm, err := services.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view clients.")
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.ClientMetadata{
		ClientId: in.ClientId,
	}

	client_metadata, err := client_info_manager.GetMetadata(ctx, in.ClientId)
	if errors.Is(err, os.ErrNotExist) {
		// Metadata not set, start with empty set.
		err = nil
	}
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	for _, i := range client_metadata.Items() {
		result.Items = append(result.Items,
			&api_proto.ClientMetadataItem{
				Key:   i.Key,
				Value: utils.ToString(i.Value),
			})
	}

	return result, nil
}

func (self *ApiServer) SetClientMetadata(
	ctx context.Context,
	in *api_proto.SetClientMetadataRequest) (*emptypb.Empty, error) {

	defer Instrument("SetClientMetadata")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user_name := user_record.Name
	permissions := acls.LABEL_CLIENT
	perm, err := services.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to modify client labels.")
	}

	client_info_manager, err := services.GetClientInfoManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	metadata := ordereddict.NewDict()
	for _, env := range in.Add {
		metadata.Set(env.Key, env.Value)
	}

	for _, key := range in.Remove {
		_, pres := metadata.Get(key)
		if !pres {
			metadata.Set(key, nil)
		}
	}

	err = client_info_manager.SetMetadata(ctx, in.ClientId, metadata, user_name)
	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) GetClient(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApiClient, error) {

	defer Instrument("GetClient")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view clients.")
	}

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Update the user's MRU
	if in.UpdateMru {
		err = indexer.UpdateMRU(org_config_obj, user_name, in.ClientId)
		if err != nil {
			return nil, Status(self.verbose, err)
		}
	}

	api_client, err := indexer.FastGetApiClient(ctx, org_config_obj, in.ClientId)
	if err != nil {
		return &api_proto.ApiClient{}, nil
	}

	if self.server_obj != nil {
		if !in.Lightweight {
			// Wait up to 2 seconds to find out if clients are connected.
			notifier, err := services.GetNotifier(org_config_obj)
			if err != nil {
				return nil, Status(self.verbose, err)
			}
			if notifier.IsClientConnected(ctx,
				org_config_obj, in.ClientId, 2) {
				api_client.LastSeenAt = uint64(time.Now().UnixNano() / 1000)
			}
		}
	}

	return api_client, nil
}
