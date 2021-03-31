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

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
)

func GetApiClient(
	ctx context.Context,
	config_obj *config_proto.Config,
	server_obj *server.Server,
	client_id string, detailed bool) (
	*api_proto.ApiClient, error) {

	if config_obj.GUI == nil {
		return nil, errors.New("GUI not configured")
	}

	result := &api_proto.ApiClient{
		ClientId: client_id,
	}

	// Special well know client id.
	if client_id == "server" {
		return result, nil
	}

	if client_id == "" || client_id[0] != 'C' {
		return nil, errors.New("client_id must start with C")
	}

	client_path_manager := paths.NewClientPathManager(client_id)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	result.Labels = services.GetLabeler().GetClientLabels(config_obj, client_id)

	client_info := &actions_proto.ClientInfo{}
	err = db.GetSubject(config_obj, client_path_manager.Path(), client_info)
	if err != nil {
		return nil, err
	}

	result.LastInterrogateFlowId = client_info.LastInterrogateFlowId
	result.AgentInformation = &api_proto.AgentInformation{
		Version: client_info.ClientVersion,
		Name:    client_info.ClientName,
	}

	result.OsInfo = &api_proto.Uname{
		System:  client_info.System,
		Release: client_info.Release,
		Machine: client_info.Architecture,
		Fqdn:    client_info.Fqdn,
	}

	public_key_info := &crypto_proto.PublicKey{}
	err = db.GetSubject(config_obj, client_path_manager.Key().Path(),
		public_key_info)
	if err != nil {
		return nil, err
	}

	result.FirstSeenAt = public_key_info.EnrollTime

	err = db.GetSubject(config_obj, client_path_manager.Ping().Path(),
		client_info)
	if err != nil {
		return nil, err
	}

	result.LastSeenAt = client_info.Ping
	result.LastIp = client_info.IpAddress

	if server_obj != nil && detailed &&
		// Wait up to 2 seconds to find out if clients are connected.
		services.GetNotifier().IsClientConnected(ctx,
			config_obj, client_id, 2) {
		result.LastSeenAt = uint64(time.Now().UnixNano() / 1000)
	}

	return result, nil
}

func (self *ApiServer) GetClientMetadata(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ClientMetadata, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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
	in *api_proto.ClientMetadata) (*empty.Empty, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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
	return &empty.Empty{}, err
}

func (self *ApiServer) GetClient(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApiClient, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view clients.")
	}

	api_client, err := GetApiClient(ctx,
		self.config,
		self.server_obj,
		in.ClientId,
		!in.Lightweight, // Detailed
	)
	if err != nil {
		return nil, err
	}

	return api_client, nil
}

func (self *ApiServer) GetClientFlows(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
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

	return flows.GetFlows(self.config, in.ClientId,
		in.IncludeArchived, filter, in.Offset, in.Count)
}
