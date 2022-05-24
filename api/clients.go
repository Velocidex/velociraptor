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
package api

import (
	"context"
	"errors"
	"os"
	"regexp"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func (self *ApiServer) GetClientMetadata(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ClientMetadata, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	if in.ClientId == "server" {
		permissions = acls.SERVER_ADMIN
	}

	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view clients.")
	}

	client_path_manager := paths.NewClientPathManager(in.ClientId)
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	result := &api_proto.ClientMetadata{}
	err = db.GetSubject(self.config, client_path_manager.Metadata(), result)
	if errors.Is(err, os.ErrNotExist) {
		// Metadata not set, start with empty set.
		err = nil
	}
	return result, err
}

func (self *ApiServer) SetClientMetadata(
	ctx context.Context,
	in *api_proto.ClientMetadata) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.LABEL_CLIENT
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to modify client labels.")
	}

	client_path_manager := paths.NewClientPathManager(in.ClientId)
	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	err = db.SetSubject(self.config, client_path_manager.Metadata(), in)
	return &emptypb.Empty{}, err
}

func (self *ApiServer) GetClient(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApiClient, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view clients.")
	}

	indexer, err := services.GetIndexer()
	if err != nil {
		return nil, err
	}

	// Update the user's MRU
	if in.UpdateMru {
		err = indexer.UpdateMRU(self.config, user_name, in.ClientId)
		if err != nil {
			return nil, err
		}
	}

	api_client, err := indexer.FastGetApiClient(ctx, self.config, in.ClientId)
	if err != nil {
		return nil, err
	}

	if self.server_obj != nil {
		if !in.Lightweight &&
			// Wait up to 2 seconds to find out if clients are connected.
			services.GetNotifier().IsClientConnected(ctx,
				self.config, in.ClientId, 2) {
			api_client.LastSeenAt = uint64(time.Now().UnixNano() / 1000)
		}
	}

	return api_client, nil
}

func (self *ApiServer) GetClientFlows(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowResponse, error) {

	users := services.GetUserManager()
	user_record, err := users.GetUserFromContext(self.config, ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view flows.")
	}

	filter := func(flow *flows_proto.ArtifactCollectorContext) bool {
		return true
	}

	if in.Artifact != "" {
		regex, err := regexp.Compile(in.Artifact)
		if err != nil {
			return nil, err
		}

		filter = func(flow *flows_proto.ArtifactCollectorContext) bool {
			if flow.Request == nil {
				return false
			}

			for _, name := range flow.Request.Artifacts {
				if regex.MatchString(name) {
					return true
				}
			}
			return false
		}
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}

	return launcher.GetFlows(self.config, in.ClientId,
		in.IncludeArchived, filter, in.Offset, in.Count)
}
