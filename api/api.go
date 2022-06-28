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
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	errors "github.com/pkg/errors"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/server"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type ApiServer struct {
	proto.UnimplementedAPIServer
	server_obj         *server.Server
	ca_pool            *x509.CertPool
	wg                 *sync.WaitGroup
	api_client_factory grpc_client.APIClientFactory
}

func (self *ApiServer) CancelFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.StartFlowResponse, error) {

	defer Instrument("CancelFlow")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name

	permissions := acls.COLLECT_CLIENT
	if in.ClientId == "server" {
		permissions = acls.COLLECT_SERVER
	}

	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to cancel flows.")
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}
	result, err := launcher.CancelFlow(
		ctx, org_config_obj, in.ClientId, in.FlowId, user_name)
	if err != nil {
		return nil, err
	}

	// Log this event as and Audit event.
	logging.GetLogger(org_config_obj, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    user_name,
			"client":  in.ClientId,
			"flow_id": in.FlowId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CancelFlow")

	return result, nil
}

func (self *ApiServer) GetReport(
	ctx context.Context,
	in *api_proto.GetReportRequest) (*api_proto.GetReportResponse, error) {

	defer Instrument("GetReport")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view reports.")
	}

	acl_manager := vql_subsystem.NewServerACLManager(org_config_obj, user_name)

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	global_repo, err := manager.GetGlobalRepository(org_config_obj)
	if err != nil {
		return nil, err
	}

	return getReport(ctx, org_config_obj, acl_manager, global_repo, in)
}

func (self *ApiServer) CollectArtifact(
	ctx context.Context,
	in *flows_proto.ArtifactCollectorArgs) (*flows_proto.ArtifactCollectorResponse, error) {

	defer Instrument("CollectArtifact")()

	result := &flows_proto.ArtifactCollectorResponse{Request: in}

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}
	creator := user_record.Name

	var acl_manager vql_subsystem.ACLManager = vql_subsystem.NullACLManager{}

	// Internal calls from the frontend can set the creator.
	if creator != org_config_obj.Client.PinnedServerName {
		in.Creator = creator

		permissions := acls.COLLECT_CLIENT
		if in.ClientId == "server" {
			permissions = acls.COLLECT_SERVER
		}

		acl_manager = vql_subsystem.NewServerACLManager(org_config_obj,
			creator)

		perm, err := acl_manager.CheckAccess(permissions)
		if !perm || err != nil {
			return nil, status.Error(codes.PermissionDenied,
				"User is not allowed to launch flows.")
		}
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(org_config_obj)
	if err != nil {
		return nil, err
	}
	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, org_config_obj, acl_manager, repository, in, nil)
	if err != nil {
		return nil, err
	}

	result.FlowId = flow_id

	// Log this event as an Audit event.
	logging.GetLogger(org_config_obj, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"client":  in.ClientId,
			"flow_id": flow_id,
			"details": fmt.Sprintf("%v", in),
		}).Info("CollectArtifact")

	return result, nil
}

func (self *ApiServer) ListClients(
	ctx context.Context,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {

	defer Instrument("ListClients")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view clients.")
	}

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return nil, err
	}

	result, err := indexer.SearchClients(ctx, org_config_obj, in, user_name)
	if err != nil {
		return nil, err
	}

	// Warm up the cache pre-emptively so we have fresh connected
	// status
	notifier := services.GetNotifier()
	for _, item := range result.Items {
		notifier.IsClientConnected(
			ctx, org_config_obj, item.ClientId, 0 /* timeout */)
	}
	return result, nil
}

func (self *ApiServer) NotifyClients(
	ctx context.Context,
	in *api_proto.NotificationRequest) (*emptypb.Empty, error) {

	defer Instrument("NotifyClients")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	notifier := services.GetNotifier()
	if notifier == nil {
		return nil, errors.New("Notifier not ready")
	}

	if in.ClientId != "" {
		self.server_obj.Info("sending notification to %s", in.ClientId)
		err = services.GetNotifier().NotifyListener(
			org_config_obj, in.ClientId, "API.NotifyClients")
	} else {
		return nil, status.Error(codes.InvalidArgument,
			"client id should be specified")
	}
	return &emptypb.Empty{}, err
}

func (self *ApiServer) LabelClients(
	ctx context.Context,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {

	defer Instrument("LabelClients")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.LABEL_CLIENT
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
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
				err = labeler.SetClientLabel(org_config_obj, client_id, label)

			case "remove":
				err = labeler.RemoveClientLabel(org_config_obj, client_id, label)

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

	defer Instrument("GetFlowDetails")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}
	result, err := launcher.GetFlowDetails(org_config_obj, in.ClientId, in.FlowId)
	return result, err
}

func (self *ApiServer) GetFlowRequests(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowRequestDetails, error) {

	defer Instrument("GetFlowRequests")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view flows.")
	}

	launcher, err := services.GetLauncher()
	if err != nil {
		return nil, err
	}
	result, err := launcher.GetFlowRequests(org_config_obj, in.ClientId, in.FlowId,
		in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetUserUITraits(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.ApiUser, error) {
	defer Instrument("GetUserUITraits")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	result := NewDefaultUserObject(org_config_obj)
	result.Username = user_info.Name
	result.InterfaceTraits.Picture = user_info.Picture
	result.InterfaceTraits.Permissions, _ = acls.GetEffectivePolicy(org_config_obj,
		result.Username)
	result.Orgs = user_info.Orgs

	for _, item := range result.Orgs {
		if item.Id == "" {
			item.Name = "<root>"
			item.Id = "root"
		}
	}

	user_options, err := users.GetUserOptions(result.Username)
	if err == nil {
		result.InterfaceTraits.Org = user_options.Org
		result.InterfaceTraits.UiSettings = user_options.Options
		result.InterfaceTraits.Theme = user_options.Theme
		result.InterfaceTraits.Timezone = user_options.Timezone
		result.InterfaceTraits.Lang = user_options.Lang
		result.InterfaceTraits.DefaultPassword = user_options.DefaultPassword
		result.InterfaceTraits.DefaultDownloadsLock = user_options.DefaultDownloadsLock
	}

	return result, nil
}

func (self *ApiServer) SetGUIOptions(
	ctx context.Context,
	in *api_proto.SetGUIOptionsRequest) (*emptypb.Empty, error) {

	users := services.GetUserManager()
	user_info, _, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	defer Instrument("SetGUIOptions")()
	return &emptypb.Empty{}, users.SetUserOptions(user_info.Name, in)
}

func (self *ApiServer) VFSListDirectory(
	ctx context.Context,
	in *api_proto.VFSListRequest) (*api_proto.VFSListResponse, error) {

	defer Instrument("VFSListDirectory")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	vfs_service, err := services.GetVFSService()
	if err != nil {
		return nil, err
	}
	result, err := vfs_service.ListDirectory(
		org_config_obj, in.ClientId, in.VfsComponents)
	return result, err
}

func (self *ApiServer) VFSStatDirectory(
	ctx context.Context,
	in *api_proto.VFSListRequest) (*api_proto.VFSListResponse, error) {

	defer Instrument("VFSStatDirectory")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	vfs_service, err := services.GetVFSService()
	if err != nil {
		return nil, err
	}

	result, err := vfs_service.StatDirectory(
		org_config_obj, in.ClientId, in.VfsComponents)
	return result, err
}

func (self *ApiServer) VFSStatDownload(
	ctx context.Context,
	in *api_proto.VFSStatDownloadRequest) (*flows_proto.VFSDownloadInfo, error) {

	defer Instrument("VFSStatDownload")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	vfs_service, err := services.GetVFSService()
	if err != nil {
		return nil, err
	}

	result, err := vfs_service.StatDownload(
		org_config_obj, in.ClientId, in.Accessor, in.Components)
	return result, err
}

func (self *ApiServer) VFSRefreshDirectory(
	ctx context.Context,
	in *api_proto.VFSRefreshDirectoryRequest) (
	*flows_proto.ArtifactCollectorResponse, error) {

	defer Instrument("VFSRefreshDirectory")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.COLLECT_CLIENT
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to launch flows.")
	}

	result, err := vfsRefreshDirectory(
		self, ctx, in.ClientId, in.VfsComponents, in.Depth)
	return result, err
}

func (self *ApiServer) VFSGetBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.VFSFileBuffer, error) {

	defer Instrument("VFSGetBuffer")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view the VFS.")
	}

	// If a client id is specified, the path is relative to the
	// client's storage directory, otherwise it is relative to the
	// root of the filestore.
	var pathspec api.FSPathSpec
	if in.ClientId != "" {
		pathspec = paths.NewClientPathManager(
			in.ClientId).FSItem(in.Components)

	} else if len(in.Components) > 0 {
		last_idx := len(in.Components) - 1
		fs_type, name := api.GetFileStorePathTypeFromExtension(
			in.Components[last_idx])
		in.Components[last_idx] = name
		pathspec = path_specs.NewUnsafeFilestorePath(in.Components...).
			SetType(fs_type)

	} else {
		return nil, status.Error(codes.InvalidArgument,
			"Invalid pathspec")
	}
	result, err := vfsGetBuffer(
		org_config_obj, in.ClientId, pathspec, in.Offset, in.Length)

	return result, err
}

func (self *ApiServer) GetTable(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetTable")()
	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view results.")
	}

	var result *api_proto.GetTableResponse

	// We want an event table.
	if in.Type == "TIMELINE" {
		result, err = getTimeline(ctx, org_config_obj, in)

	} else if in.Type == "CLIENT_EVENT_LOGS" || in.Type == "SERVER_EVENT_LOGS" {
		result, err = getEventTableLogs(ctx, org_config_obj, in)

	} else if in.Type == "CLIENT_EVENT" || in.Type == "SERVER_EVENT" {
		result, err = getEventTable(ctx, org_config_obj, in)

	} else {
		result, err = getTable(ctx, org_config_obj, in)
	}

	if err != nil {
		return nil, err
	}

	if in.Artifact != "" {
		manager, err := services.GetRepositoryManager()
		if err != nil {
			return nil, err
		}

		repository, err := manager.GetGlobalRepository(org_config_obj)
		if err != nil {
			return nil, err
		}

		artifact, pres := repository.Get(org_config_obj, in.Artifact)
		if pres {
			result.ColumnTypes = artifact.ColumnTypes
		}
	}
	return result, nil
}

func (self *ApiServer) GetArtifacts(
	ctx context.Context,
	in *api_proto.GetArtifactsRequest) (
	*artifacts_proto.ArtifactDescriptors, error) {

	defer Instrument("GetArtifacts")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view custom artifacts.")
	}

	if len(in.Names) > 0 {
		result := &artifacts_proto.ArtifactDescriptors{}
		manager, err := services.GetRepositoryManager()
		if err != nil {
			return nil, err
		}

		repository, err := manager.GetGlobalRepository(org_config_obj)
		if err != nil {
			return nil, err
		}

		for _, name := range in.Names {
			artifact, pres := repository.Get(org_config_obj, name)
			if pres {
				result.Items = append(result.Items, artifact)
			}
		}
		return result, nil
	}

	if in.ReportType != "" {
		return getReportArtifacts(
			ctx, org_config_obj, in.ReportType, in.NumberOfResults)
	}

	terms := strings.Split(in.SearchTerm, " ")
	result, err := searchArtifact(
		ctx, org_config_obj, terms, in.Type, in.NumberOfResults, in.Fields)
	return result, err
}

func (self *ApiServer) GetArtifactFile(
	ctx context.Context,
	in *api_proto.GetArtifactRequest) (
	*api_proto.GetArtifactResponse, error) {

	defer Instrument("GetArtifactFile")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied,
			"User is not allowed to view custom artifacts.")
	}

	artifact, err := getArtifactFile(org_config_obj, in.Name)
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

	defer Instrument("SetArtifactFile")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.ARTIFACT_WRITER

	// First ensure that the artifact is correct.
	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	tmp_repository := manager.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		in.Artifact, true /* validate */, false /* built_in */)
	if err != nil {
		return nil, err
	}

	switch strings.ToUpper(artifact_definition.Type) {
	case "CLIENT", "CLIENT_EVENT":
		permissions = acls.ARTIFACT_WRITER
	case "SERVER", "SERVER_EVENT":
		permissions = acls.SERVER_ARTIFACT_WRITER
	}

	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify artifacts (%v).", permissions))
	}

	definition, err := setArtifactFile(org_config_obj, user_name, in, "")
	if err != nil {
		message := &api_proto.APIResponse{
			Error:        true,
			ErrorMessage: fmt.Sprintf("%v", err),
		}
		return message, errors.New(message.ErrorMessage)
	}

	logging.GetLogger(org_config_obj, &logging.Audit).
		WithFields(logrus.Fields{
			"user":     user_name,
			"artifact": definition.Name,
			"details":  fmt.Sprintf("%v", in.Artifact),
		}).Info("SetArtifactFile")

	return &api_proto.APIResponse{}, nil
}

func (self *ApiServer) Query(
	in *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer) error {

	defer Instrument("Query")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(stream.Context())
	if err != nil {
		return err
	}

	user_name := user_info.Name

	// Check that the principal is allowed to issue queries.
	permissions := acls.ANY_QUERY
	ok, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if err != nil {
		return status.Error(codes.PermissionDenied,
			fmt.Sprintf("User %v is not allowed to run queries.",
				user_name))
	}

	if !ok {
		return status.Error(codes.PermissionDenied, fmt.Sprintf(
			"Permission denied: User %v requires permission %v to run queries",
			user_name, permissions))
	}

	return streamQuery(stream.Context(), org_config_obj, in, stream, user_name)
}

func (self *ApiServer) GetServerMonitoringState(
	ctx context.Context,
	in *emptypb.Empty) (
	*flows_proto.ArtifactCollectorArgs, error) {

	defer Instrument("GetServerMonitoringState")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.READ_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to read results (%v).", permissions))
	}

	result, err := getServerMonitoringState(org_config_obj)
	return result, err
}

func (self *ApiServer) SetServerMonitoringState(
	ctx context.Context,
	in *flows_proto.ArtifactCollectorArgs) (
	*flows_proto.ArtifactCollectorArgs, error) {

	defer Instrument("SetServerMonitoringState")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify artifacts (%v).", permissions))
	}

	err = setServerMonitoringState(org_config_obj, user_name, in)
	return in, err
}

func (self *ApiServer) GetClientMonitoringState(
	ctx context.Context, in *flows_proto.GetClientMonitoringStateRequest) (
	*flows_proto.ClientEventTable, error) {

	defer Instrument("GetClientMonitoringState")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to read monitoring artifacts (%v).", permissions))
	}

	manager := services.ClientEventManager()
	result := manager.GetClientMonitoringState()
	if in.ClientId != "" {
		message := manager.GetClientUpdateEventTableMessage(org_config_obj,
			in.ClientId)
		result.ClientMessage = message
	}

	return result, err
}

func (self *ApiServer) SetClientMonitoringState(
	ctx context.Context,
	in *flows_proto.ClientEventTable) (
	*emptypb.Empty, error) {

	defer Instrument("SetClientMonitoringState")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.SERVER_ADMIN
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to modify monitoring artifacts (%v).", permissions))
	}

	err = services.ClientEventManager().SetClientMonitoringState(
		ctx, org_config_obj, user_name, in)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, err
}

func (self *ApiServer) CreateDownloadFile(ctx context.Context,
	in *api_proto.CreateDownloadRequest) (*api_proto.CreateDownloadResponse, error) {

	defer Instrument("CreateDownloadFile")()

	users := services.GetUserManager()
	user_info, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	user_name := user_info.Name
	permissions := acls.PREPARE_RESULTS
	perm, err := acls.CheckAccess(org_config_obj, user_name, permissions)
	if !perm || err != nil {
		return nil, status.Error(codes.PermissionDenied, fmt.Sprintf(
			"User is not allowed to create downloads (%v).", permissions))
	}

	// Log an audit event.
	logging.GetLogger(org_config_obj, &logging.Audit).
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
		query = `SELECT create_flow_download(password=Password,
      client_id=ClientId, flow_id=FlowId, type=DownloadType) AS VFSPath
      FROM scope()`

		env.Set("ClientId", in.ClientId).
			Set("FlowId", in.FlowId).
			Set("Password", in.Password).
			Set("DownloadType", in.DownloadType)

	} else if in.HuntId != "" {
		query = `SELECT create_hunt_download(password=Password,
      hunt_id=HuntId, only_combined=OnlyCombined, format=Format) AS VFSPath
      FROM scope()`

		env.Set("HuntId", in.HuntId).
			Set("Format", format).
			Set("Password", in.Password).
			Set("OnlyCombined", in.OnlyCombinedHunt)
	}

	manager, err := services.GetRepositoryManager()
	if err != nil {
		return nil, err
	}

	scope := manager.BuildScope(
		services.ScopeBuilder{
			Config:     org_config_obj,
			Env:        env,
			ACLManager: vql_subsystem.NewServerACLManager(org_config_obj, user_name),
			Logger:     logging.NewPlainLogger(org_config_obj, &logging.FrontendComponent),
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

	if config_obj.API == nil ||
		config_obj.Client == nil ||
		config_obj.Frontend == nil {
		return errors.New("API server not configured")
	}

	bind_addr := config_obj.API.BindAddress
	switch config_obj.API.BindScheme {
	case "tcp":
		bind_addr += fmt.Sprintf(":%d", config_obj.API.BindPort)
	}

	lis, err := net.Listen(config_obj.API.BindScheme, bind_addr)
	if err != nil {
		return errors.WithStack(err)
	}

	// Use the server certificate to secure the gRPC connection.
	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return errors.WithStack(err)
	}

	// Authenticate API clients using certificates.
	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	// Create the TLS credentials
	creds := credentials.NewTLS(&tls.Config{
		// Only accept certs signed by the CA
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    CA_Pool,
	})

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	api_proto.RegisterAPIServer(
		grpcServer,
		&ApiServer{
			server_obj:         server_obj,
			ca_pool:            CA_Pool,
			api_client_factory: grpc_client.GRPCAPIClient{},
			wg:                 wg,
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
	config_obj *config_proto.Config) error {

	if config_obj.Monitoring == nil {
		return nil
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	env_inject_time, pres := os.LookupEnv("VELOCIRAPTOR_INJECT_API_SLEEP")
	if pres {
		logger.Info("Injecting delays for API calls since VELOCIRAPTOR_INJECT_API_SLEEP is set (only used for testing).")
		result, err := strconv.ParseInt(env_inject_time, 0, 64)
		if err == nil {
			inject_time = int(result)
		}
	}

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
	return nil
}
