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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	users "www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ApiServer struct {
	config     *config_proto.Config
	server_obj *server.Server
	ca_pool    *x509.CertPool

	api_client_factory grpc_client.APIClientFactory
}

func (self *ApiServer) CancelFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.StartFlowResponse, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name

	permissions := acls.COLLECT_CLIENT
	if in.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to cancel flows.")
	}

	result, err := flows.CancelFlow(
		ctx,
		self.config, in.ClientId, in.FlowId, user_name,
		self.api_client_factory)
	if err != nil {
		return nil, err
	}

	// Log this event as and Audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    user_name,
			"client":  in.ClientId,
			"flow_id": in.FlowId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CancelFlow")

	return result, nil
}

func (self *ApiServer) ArchiveFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.StartFlowResponse, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name

	permissions := acls.COLLECT_CLIENT
	if in.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to archive flows.")
	}

	result, err := flows.ArchiveFlow(self.config, in.ClientId, in.FlowId, user_name)
	if err != nil {
		return nil, err
	}

	// Log this event as and Audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    user_name,
			"client":  in.ClientId,
			"flow_id": in.FlowId,
			"details": fmt.Sprintf("%v", in),
		}).Info("ArchiveFlow")

	return result, nil
}

func (self *ApiServer) GetReport(
	ctx context.Context,
	in *api_proto.GetReportRequest) (*api_proto.GetReportResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view reports.")
	}

	acl_manager := vql_subsystem.NewServerACLManager(self.config, user_name)

	global_repo, err := services.GetRepositoryManager().GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}

	return getReport(ctx, self.config, acl_manager, global_repo, in)
}

func (self *ApiServer) CollectArtifact(
	ctx context.Context,
	in *flows_proto.ArtifactCollectorArgs) (*flows_proto.ArtifactCollectorResponse, error) {
	result := &flows_proto.ArtifactCollectorResponse{Request: in}
	creator := GetGRPCUserInfo(self.config, ctx).Name

	var acl_manager vql_subsystem.ACLManager = vql_subsystem.NullACLManager{}

	// Internal calls from the frontend can set the creator.
	if creator != self.config.Client.PinnedServerName {
		in.Creator = creator

		permissions := acls.COLLECT_CLIENT
		if in.ClientId == "server" {
			permissions = acls.COLLECT_SERVER
		}

		acl_manager = vql_subsystem.NewServerACLManager(self.config,
			creator)

		perm, err := acl_manager.CheckAccess(permissions)
		if !perm || err != nil {
			return nil, status.Error(codes.PermissionDenied,
				"User is not allowed to launch flows.")
		}
	}

	repository, err := services.GetRepositoryManager().GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}

	flow_id, err := services.GetLauncher().ScheduleArtifactCollection(
		ctx, self.config, acl_manager, repository, in)
	if err != nil {
		return nil, err
	}

	result.FlowId = flow_id

	err = services.GetNotifier().NotifyListener(self.config, in.ClientId)
	if err != nil {
		return nil, err
	}

	// Log this event as an Audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"client":  in.ClientId,
			"flow_id": flow_id,
			"details": fmt.Sprintf("%v", in),
		}).Info("CollectArtifact")

	return result, nil
}

func (self *ApiServer) CreateHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*api_proto.StartFlowResponse, error) {

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(self.config, ctx).Name
	in.HuntId = flows.GetNewHuntId()

	acl_manager := vql_subsystem.NewServerACLManager(self.config, in.Creator)

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch hunts.")
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CreateHunt")

	result := &api_proto.StartFlowResponse{}
	hunt_id, err := flows.CreateHunt(
		ctx, self.config, acl_manager, in)
	if err != nil {
		return nil, err
	}

	result.FlowId = hunt_id

	return result, nil
}

func (self *ApiServer) ModifyHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*empty.Empty, error) {

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(self.config, ctx).Name

	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, in.Creator, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to modify hunts.")
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("ModifyHunt")

	err = flows.ModifyHunt(ctx, self.config, in, in.Creator)
	if err != nil {
		return nil, err
	}

	result := &empty.Empty{}
	return result, nil
}

func (self *ApiServer) ListHunts(
	ctx context.Context,
	in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	result, err := flows.ListHunts(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetHunt(
	ctx context.Context,
	in *api_proto.GetHuntRequest) (*api_proto.Hunt, error) {
	if in.HuntId == "" {
		return &api_proto.Hunt{}, nil
	}

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view hunts.")
	}

	result, err := flows.GetHunt(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetHuntResults(
	ctx context.Context,
	in *api_proto.GetHuntResultsRequest) (*api_proto.GetTableResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	env := ordereddict.NewDict().
		Set("HuntID", in.HuntId).
		Set("Artifact", in.Artifact)

	// More than 100 results are not very useful in the GUI -
	// users should just download the json file for post
	// processing or process in the notebook.
	result, err := RunVQL(ctx, self.config, user_name, env,
		"SELECT * FROM hunt_results(hunt_id=HuntID, "+
			"artifact=Artifact, source=Source) LIMIT 100")
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) ListClients(
	ctx context.Context,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name

	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view clients.")
	}

	db, err := datastore.GetDB(self.config)
	if err != nil {
		return nil, err
	}

	limit := uint64(50)
	if in.Limit > 0 {
		limit = in.Limit
	}

	query_type := ""
	if in.Type == api_proto.SearchClientsRequest_KEY {
		query_type = "key"
	}

	result := &api_proto.SearchClientsResponse{}
	for _, client_id := range db.SearchClients(
		self.config, constants.CLIENT_INDEX_URN,
		in.Query, query_type, in.Offset, limit) {
		if in.NameOnly || query_type == "key" {
			result.Names = append(result.Names, client_id)
		} else {
			api_client, err := GetApiClient(
				self.config, self.server_obj, client_id, false)
			if err == nil {
				result.Items = append(result.Items, api_client)
			}
		}
	}

	return result, nil
}

func (self *ApiServer) NotifyClients(
	ctx context.Context,
	in *api_proto.NotificationRequest) (*empty.Empty, error) {
	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	if in.NotifyAll {
		self.server_obj.Info("sending notification to everyone")
		err = services.GetNotifier().NotifyAllListeners(self.config)
	} else if in.ClientId != "" {
		self.server_obj.Info("sending notification to %s", in.ClientId)
		err = services.GetNotifier().NotifyListener(self.config, in.ClientId)
	} else {
		return nil, status.Error(codes.InvalidArgument,
			"client id should be specified")
	}
	return &empty.Empty{}, err
}

func (self *ApiServer) LabelClients(
	ctx context.Context,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.LABEL_CLIENT
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return &api_proto.APIResponse{
				Error:        true,
				ErrorMessage: "Permission Denied",
			}, status.Error(codes.PermissionDenied,
				"User is not allowed to label clients.")
	}

	labeler := services.GetLabeler()
	for _, client_id := range in.ClientIds {
		for _, label := range in.Labels {
			switch in.Operation {
			case "set":
				err = labeler.SetClientLabel(self.config, client_id, label)

			case "remove":
				err = labeler.RemoveClientLabel(self.config, client_id, label)

			default:
				return nil, errors.New("Unknown label operation")
			}

			if err != nil {
				return &api_proto.APIResponse{
					Error:        true,
					ErrorMessage: err.Error(),
				}, err
			}
		}
	}

	return &api_proto.APIResponse{}, nil
}

func (self *ApiServer) GetFlowDetails(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.FlowDetails, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	result, err := flows.GetFlowDetails(self.config, in.ClientId, in.FlowId)
	return result, err
}

func (self *ApiServer) GetFlowRequests(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowRequestDetails, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view flows.")
	}

	result, err := flows.GetFlowRequests(self.config, in.ClientId, in.FlowId,
		in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetUserUITraits(
	ctx context.Context,
	in *empty.Empty) (*api_proto.ApiGrrUser, error) {
	result := NewDefaultUserObject(self.config)
	user_info := GetGRPCUserInfo(self.config, ctx)

	result.Username = user_info.Name
	result.InterfaceTraits.Picture = user_info.Picture
	result.InterfaceTraits.Permissions, _ = acls.GetEffectivePolicy(self.config,
		result.Username)

	user_options, err := users.GetUserOptions(self.config, result.Username)
	if err == nil {
		result.InterfaceTraits.UiSettings = user_options.Options
	}

	return result, nil
}

func (self *ApiServer) SetGUIOptions(
	ctx context.Context,
	in *api_proto.SetGUIOptionsRequest) (*empty.Empty, error) {
	user_info := GetGRPCUserInfo(self.config, ctx)

	return &empty.Empty{}, users.SetUserOptions(self.config, user_info.Name, in)
}

func (self *ApiServer) GetUserNotifications(
	ctx context.Context,
	in *api_proto.GetUserNotificationsRequest) (
	*api_proto.GetUserNotificationsResponse, error) {
	result, err := users.GetUserNotifications(
		self.config, GetGRPCUserInfo(self.config, ctx).Name, in.ClearPending)
	return result, err
}

func (self *ApiServer) GetUserNotificationCount(
	ctx context.Context,
	in *empty.Empty) (*api_proto.UserNotificationCount, error) {
	n, err := users.GetUserNotificationCount(
		self.config, GetGRPCUserInfo(self.config, ctx).Name)
	return &api_proto.UserNotificationCount{Count: n}, err
}

func (self *ApiServer) VFSListDirectory(
	ctx context.Context,
	in *flows_proto.VFSListRequest) (*flows_proto.VFSListResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	result, err := vfsListDirectory(
		self.config, in.ClientId, in.VfsPath)
	return result, err
}

func (self *ApiServer) VFSStatDirectory(
	ctx context.Context,
	in *flows_proto.VFSListRequest) (*flows_proto.VFSListResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	result, err := vfsStatDirectory(
		self.config, in.ClientId, in.VfsPath)
	return result, err
}

func (self *ApiServer) VFSStatDownload(
	ctx context.Context,
	in *flows_proto.VFSStatDownloadRequest) (*flows_proto.VFSDownloadInfo, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	result, err := vfsStatDownload(
		self.config, in.ClientId, in.Accessor, in.Path)
	return result, err
}

func (self *ApiServer) VFSRefreshDirectory(
	ctx context.Context,
	in *api_proto.VFSRefreshDirectoryRequest) (
	*flows_proto.ArtifactCollectorResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	result, err := vfsRefreshDirectory(
		self, ctx, in.ClientId, in.VfsPath, in.Depth)
	return result, err
}

func (self *ApiServer) VFSGetBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.VFSFileBuffer, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	result, err := vfsGetBuffer(
		self.config, in.ClientId, in.VfsPath, in.Offset, in.Length)

	return result, err
}

func (self *ApiServer) GetTable(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	result, err := getTable(ctx, self.config, in)
	if err != nil {
		return &api_proto.GetTableResponse{}, nil
	}
	return result, nil
}

func (self *ApiServer) GetArtifacts(
	ctx context.Context,
	in *api_proto.GetArtifactsRequest) (
	*artifacts_proto.ArtifactDescriptors, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view custom artifacts.")
	}

	if len(in.Names) > 0 {
		result := &artifacts_proto.ArtifactDescriptors{}
		repository, err := services.GetRepositoryManager().GetGlobalRepository(self.config)
		if err != nil {
			return nil, err
		}

		for _, name := range in.Names {
			artifact, pres := repository.Get(self.config, name)
			if pres {
				result.Items = append(result.Items, artifact)
			}
		}
		return result, nil
	}

	if in.ReportType != "" {
		return getReportArtifacts(
			self.config, in.ReportType, in.NumberOfResults)
	}

	terms := strings.Split(in.SearchTerm, " ")
	result, err := searchArtifact(
		self.config, terms, in.Type, in.NumberOfResults)
	return result, err
}

func (self *ApiServer) GetArtifactFile(
	ctx context.Context,
	in *api_proto.GetArtifactRequest) (
	*api_proto.GetArtifactResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view custom artifacts.")
	}

	artifact, err := getArtifactFile(self.config, in.Name)
	if err != nil {
		return nil, err
	}

	result := &api_proto.GetArtifactResponse{
		Artifact: artifact,
	}
	return result, nil
}

func (self *ApiServer) SetArtifactFile(
	ctx context.Context,
	in *api_proto.SetArtifactRequest) (
	*api_proto.APIResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.ARTIFACT_WRITER

	// First ensure that the artifact is correct.
	tmp_repository := services.GetRepositoryManager().NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		in.Artifact, true /* validate */)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(artifact_definition.Type) {
	case "CLIENT", "CLIENT_EVENT":
		permissions = acls.ARTIFACT_WRITER
	case "SERVER", "SERVER_EVENT":
		permissions = acls.SERVER_ARTIFACT_WRITER
	}

	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify artifacts (%v).", permissions))
	}

	definition, err := setArtifactFile(self.config, in,
		constants.ARTIFACT_CUSTOM_NAME_PREFIX /* required_prefix */)
	if err != nil {
		return &api_proto.APIResponse{
			Error:        true,
			ErrorMessage: fmt.Sprintf("%v", err),
		}, nil
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":     user_name,
			"artifact": definition.Name,
			"details":  fmt.Sprintf("%v", in.Artifact),
		}).Info("SetArtifactFile")

	return &api_proto.APIResponse{}, nil
}

func (self *ApiServer) WriteEvent(
	ctx context.Context,
	in *actions_proto.VQLResponse) (*empty.Empty, error) {

	// Get the TLS context from the peer and verify its
	// certificate.
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return nil, err
		}

		if len(chains) == 0 {
			return nil, status.Error(codes.InvalidArgument, "no chains verified")
		}

		peer_name := peer_cert.Subject.CommonName

		token, err := acls.GetEffectivePolicy(self.config, peer_name)
		if err != nil {
			return nil, err
		}

		// Check that the principal is allowed to push to the queue.
		ok, err := acls.CheckAccessWithToken(token, acls.PUBLISH, in.Query.Name)
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, status.Error(codes.PermissionDenied,
				"Permission denied: PUBLISH "+peer_name+" to "+in.Query.Name)
		}

		rows, err := utils.ParseJsonToDicts([]byte(in.Response))
		if err != nil {
			return nil, err
		}

		// Only return the first row
		if true {
			err := services.GetJournal().PushRowsToArtifact(self.config,
				rows, in.Query.Name, peer_name, "")
			return &empty.Empty{}, err
		}
	}

	return nil, status.Error(codes.InvalidArgument, "no peer certs?")
}

func (self *ApiServer) Query(
	in *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer) error {

	// Get the TLS context from the peer and verify its
	// certificate.
	peer, ok := peer.FromContext(stream.Context())
	if !ok {
		return status.Error(codes.InvalidArgument, "cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return status.Error(codes.InvalidArgument, "unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return err
		}

		if len(chains) == 0 {
			return status.Error(codes.InvalidArgument, "no chains verified")
		}

		peer_name := peer_cert.Subject.CommonName

		// Check that the principal is allowed to issue queries.
		permissions := acls.ANY_QUERY
		ok, err := acls.CheckAccess(self.config, peer_name, permissions)
		if err != nil {
			return status.Error(codes.PermissionDenied,
				fmt.Sprintf("User %v is not allowed to run queries.",
					peer_name))
		}

		if !ok {
			return status.Error(codes.PermissionDenied, fmt.Sprintf(
				"Permission denied: User %v requires permission %v to run queries",
				peer_name, permissions))
		}

		// return the first good match
		if true {
			// Cert is good enough for us, run the query.
			return streamQuery(self.config, in, stream, peer_name)
		}
	}

	return status.Error(codes.InvalidArgument, "no peer certs?")
}

func (self *ApiServer) GetServerMonitoringState(
	ctx context.Context,
	in *empty.Empty) (
	*flows_proto.ArtifactCollectorArgs, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to read results (%v).", permissions))
	}

	result, err := getServerMonitoringState(self.config)
	return result, err
}

func (self *ApiServer) SetServerMonitoringState(
	ctx context.Context,
	in *flows_proto.ArtifactCollectorArgs) (
	*flows_proto.ArtifactCollectorArgs, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify artifacts (%v).", permissions))
	}

	err = setServerMonitoringState(self.config, in)
	return in, err
}

func (self *ApiServer) GetClientMonitoringState(
	ctx context.Context, in *empty.Empty) (
	*flows_proto.ClientEventTable, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to read monitoring artifacts (%v).", permissions))
	}

	result := services.ClientEventManager().GetClientMonitoringState()
	return result, err
}

func (self *ApiServer) SetClientMonitoringState(
	ctx context.Context,
	in *flows_proto.ClientEventTable) (
	*empty.Empty, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify monitoring artifacts (%v).", permissions))
	}

	err = services.ClientEventManager().SetClientMonitoringState(ctx, self.config, in)
	if err != nil {
		return nil, err
	}

	_, err = self.NotifyClients(ctx, &api_proto.NotificationRequest{
		NotifyAll: true,
	})

	return &empty.Empty{}, err
}

func (self *ApiServer) CreateDownloadFile(ctx context.Context,
	in *api_proto.CreateDownloadRequest) (*api_proto.CreateDownloadResponse, error) {

	user_name := GetGRPCUserInfo(self.config, ctx).Name
	permissions := acls.PREPARE_RESULTS
	perm, err := acls.CheckAccess(self.config, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to create downloads (%v).", permissions))
	}

	// Log an audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    user_name,
			"request": in,
		}).Info("CreateDownloadRequest")

	format := ""
	if in.JsonFormat {
		format = "json"
	} else if in.CsvFormat {
		format = "csv"
	}

	query := ""
	env := ordereddict.NewDict()
	if in.FlowId != "" && in.ClientId != "" {
		query = `SELECT create_flow_download(
      client_id=ClientId, flow_id=FlowId, type=DownloadType) AS VFSPath
      FROM scope()`

		env.Set("ClientId", in.ClientId).
			Set("FlowId", in.FlowId).
			Set("DownloadType", in.DownloadType)

	} else if in.HuntId != "" {
		query = `SELECT create_hunt_download(
      hunt_id=HuntId, only_combined=OnlyCombined, format=Format) AS VFSPath
      FROM scope()`

		env.Set("HuntId", in.HuntId).
			Set("Format", format).
			Set("OnlyCombined", in.OnlyCombinedHunt)

	}

	scope := services.GetRepositoryManager().BuildScope(
		services.ScopeBuilder{
			Config:     self.config,
			Env:        env,
			ACLManager: vql_subsystem.NewServerACLManager(self.config, user_name),
			Logger:     logging.NewPlainLogger(self.config, &logging.FrontendComponent),
		})
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	if err != nil {
		return nil, err
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	result := &api_proto.CreateDownloadResponse{}
	for row := range vql.Eval(sub_ctx, scope) {
		result.VfsPath = vql_subsystem.GetStringFromRow(scope, row, "VFSPath")
	}

	return result, err
}

func startAPIServer(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config,
	server_obj *server.Server) error {
	bind_addr := config_obj.API.BindAddress
	switch config_obj.API.BindScheme {
	case "tcp":
		bind_addr += fmt.Sprintf(":%d", config_obj.API.BindPort)
	}

	lis, err := net.Listen(config_obj.API.BindScheme, bind_addr)
	if err != nil {
		return err
	}

	// Use the server certificate to secure the gRPC connection.
	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return err
	}

	// Authenticate API clients using certificates.
	CA_Pool := x509.NewCertPool()
	CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))

	// Create the TLS credentials
	creds := credentials.NewTLS(&tls.Config{
		// We verify the cert ourselves in the handler.
		ClientAuth:   tls.RequireAnyClientCert,
		Certificates: []tls.Certificate{cert},
	})

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	api_proto.RegisterAPIServer(
		grpcServer,
		&ApiServer{
			config:             config_obj,
			server_obj:         server_obj,
			ca_pool:            CA_Pool,
			api_client_factory: grpc_client.GRPCAPIClient{},
		},
	)
	// Register reflection service.
	reflection.Register(grpcServer)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> gRPC API server on %v ", bind_addr)

	wg.Add(1)
	go func() {
		defer wg.Done()

		err = grpcServer.Serve(lis)
		if err != nil {
			logger.Error("gRPC Server error: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()
		logger.Info("<red>Shutting down</> gRPC API server")
		grpcServer.Stop()
	}()

	return nil
}

func StartMonitoringService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	bind_addr := fmt.Sprintf("%s:%d",
		config_obj.Monitoring.BindAddress,
		config_obj.Monitoring.BindPort)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:     bind_addr,
		Handler:  mux,
		ErrorLog: logging.NewPlainLogger(config_obj, &logging.FrontendComponent),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Prometheus monitoring server: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Wait for context to become cancelled.
		<-ctx.Done()

		logger.Info("<red>Shutting down</> Prometheus monitoring service")
		timeout_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("Prometheus shutdown error: %v", err)
		}
	}()

	logger.Info("Launched Prometheus monitoring server on %v ", bind_addr)
}
