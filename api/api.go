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

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"www.velocidex.com/golang/velociraptor/acls"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/api/authenticators"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/api/tables"
	api_utils "www.velocidex.com/golang/velociraptor/api/utils"
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
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
)

type ApiServer struct {
	api_proto.UnimplementedAPIServer
	server_obj         *server.Server
	ca_pool            *x509.CertPool
	wg                 *sync.WaitGroup
	verbose            bool
	api_client_factory grpc_client.APIClientFactory
}

func (self *ApiServer) GetReport(
	ctx context.Context,
	in *api_proto.GetReportRequest) (*api_proto.GetReportResponse, error) {

	defer Instrument("GetReport")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name
	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view reports.")
	}

	acl_manager := acl_managers.NewServerACLManager(org_config_obj, principal)

	manager, err := services.GetRepositoryManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	global_repo, err := manager.GetGlobalRepository(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
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
		return nil, Status(self.verbose, err)
	}

	// Build a request based on user input.
	request := &flows_proto.ArtifactCollectorArgs{
		ClientId:        in.ClientId,
		Artifacts:       in.Artifacts,
		Specs:           in.Specs,
		Creator:         user_record.Name,
		OpsPerSecond:    in.OpsPerSecond,
		CpuLimit:        in.CpuLimit,
		IopsLimit:       in.IopsLimit,
		Timeout:         in.Timeout,
		ProgressTimeout: in.ProgressTimeout,
		MaxRows:         in.MaxRows,
		MaxLogs:         in.MaxLogs,
		MaxUploadBytes:  in.MaxUploadBytes,
		Urgent:          in.Urgent,
		TraceFreqSec:    in.TraceFreqSec,
	}

	acl_manager := acl_managers.NewServerACLManager(
		org_config_obj, user_record.Name)

	manager, err := services.GetRepositoryManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	repository, err := manager.GetGlobalRepository(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	flow_id, err := launcher.ScheduleArtifactCollection(
		ctx, org_config_obj, acl_manager, repository, request,
		utils.BackgroundWriter)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result.FlowId = flow_id

	// Log this event as an Audit event.
	err = services.LogAudit(ctx,
		org_config_obj, request.Creator, "ScheduleFlow",
		ordereddict.NewDict().
			Set("client", request.ClientId).
			Set("flow_id", flow_id).
			Set("details", request))

	return result, err
}

func (self *ApiServer) ListClients(
	ctx context.Context,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {

	defer Instrument("ListClients")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view clients.")
	}

	indexer, err := services.GetIndexer(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, err := indexer.SearchClients(ctx, org_config_obj, in, principal)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// Warm up the cache pre-emptively so we have fresh connected
	// status
	notifier, err := services.GetNotifier(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
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
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.COLLECT_CLIENT
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to launch flows.")
	}

	notifier, err := services.GetNotifier(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	if in.ClientId != "" {
		self.server_obj.Info("sending notification to %s", in.ClientId)
		err = notifier.NotifyListener(ctx, org_config_obj, in.ClientId,
			"API.NotifyClients")
	} else {
		return nil, status.Error(codes.InvalidArgument,
			"client id should be specified")
	}
	return &emptypb.Empty{}, Status(self.verbose, err)
}

func (self *ApiServer) LabelClients(
	ctx context.Context,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {

	defer Instrument("LabelClients")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name
	permissions := acls.LABEL_CLIENT
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return &api_proto.APIResponse{
				Error:        true,
				ErrorMessage: "Permission Denied",
			}, status.Error(codes.PermissionDenied,
				"User is not allowed to label clients.")
	}

	labeler := services.GetLabeler(org_config_obj)
	for _, client_id := range in.ClientIds {
		for _, label := range in.Labels {
			switch in.Operation {
			case "set":
				err = labeler.SetClientLabel(ctx,
					org_config_obj, client_id, label)
				if err == nil {
					err := services.LogAudit(ctx,
						org_config_obj, principal, "SetClientLabel",
						ordereddict.NewDict().
							Set("client_id", client_id).
							Set("label", label))
					if err != nil {
						return nil, Status(self.verbose, err)
					}
				}

			case "remove":
				err = labeler.RemoveClientLabel(ctx,
					org_config_obj, client_id, label)
				if err == nil {
					err := services.LogAudit(ctx,
						org_config_obj, principal, "RemoveClientLabel",
						ordereddict.NewDict().
							Set("client_id", client_id).
							Set("label", label))
					if err != nil {
						return nil, Status(self.verbose, err)
					}
				}

			default:
				return nil, errors.New("Unknown label operation")
			}

			if err != nil {
				return &api_proto.APIResponse{
					Error:        true,
					ErrorMessage: err.Error(),
				}, Status(self.verbose, err)
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
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to launch flows.")
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	result, err := launcher.GetFlowDetails(
		ctx, org_config_obj, services.GetFlowOptions{Downloads: true},
		in.ClientId, in.FlowId)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	return result, nil
}

func (self *ApiServer) GetFlowRequests(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowRequestDetails, error) {

	defer Instrument("GetFlowRequests")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view flows.")
	}

	launcher, err := services.GetLauncher(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	result, err := launcher.Storage().GetFlowRequests(
		ctx, org_config_obj, in.ClientId, in.FlowId, in.Offset, in.Count)
	return result, Status(self.verbose, err)
}

func (self *ApiServer) GetUserUITraits(
	ctx context.Context,
	in *emptypb.Empty) (*api_proto.ApiUser, error) {
	defer Instrument("GetUserUITraits")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	authenticator, err := authenticators.NewAuthenticator(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := NewDefaultUserObject(org_config_obj)
	result.Username = principal
	result.InterfaceTraits.PasswordLess = authenticator.IsPasswordLess()
	result.InterfaceTraits.AuthRedirectTemplate = authenticator.AuthRedirectTemplate()
	result.InterfaceTraits.Picture = user_record.Picture
	result.InterfaceTraits.Permissions, _ = services.GetEffectivePolicy(org_config_obj,
		result.Username)
	result.Orgs = user_record.Orgs

	for _, item := range result.Orgs {
		if utils.IsRootOrg(item.Id) {
			item.Name = "<root>"
			item.Id = "root"
		}
	}

	user_options, err := users.GetUserOptions(ctx, result.Username)
	if err == nil {
		result.InterfaceTraits.Org = user_options.Org
		result.InterfaceTraits.UiSettings = user_options.Options
		result.InterfaceTraits.Theme = user_options.Theme
		result.InterfaceTraits.Timezone = user_options.Timezone
		result.InterfaceTraits.Lang = user_options.Lang
		result.InterfaceTraits.DefaultPassword = user_options.DefaultPassword
		result.InterfaceTraits.DefaultDownloadsLock = user_options.DefaultDownloadsLock
		result.InterfaceTraits.Customizations = user_options.Customizations
		result.InterfaceTraits.Links = user_options.Links
		result.InterfaceTraits.DisableServerEvents = user_options.DisableServerEvents
		result.InterfaceTraits.DisableQuarantineButton = user_options.DisableQuarantineButton

		frontend_service, err := services.GetFrontendManager(org_config_obj)
		if err == nil {
			url, err := frontend_service.GetBaseURL(org_config_obj)
			if err == nil {
				result.InterfaceTraits.BasePath = url.Path
			}

			result.GlobalMessages = frontend_service.GetGlobalMessages()
		}
	}

	return result, nil
}

func (self *ApiServer) SetGUIOptions(
	ctx context.Context,
	in *api_proto.SetGUIOptionsRequest) (*api_proto.SetGUIOptionsResponse, error) {

	defer Instrument("SetGUIOptions")()

	users := services.GetUserManager()
	user_record, _, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	// This API is only used for the user to change their own options
	// so it is always allowed.
	return &api_proto.SetGUIOptionsResponse{},
		users.SetUserOptions(ctx, principal, principal, in)
}

// Only list the child directories - used by the tree widget.
func (self *ApiServer) VFSListDirectory(
	ctx context.Context,
	in *api_proto.VFSListRequest) (*api_proto.VFSListResponse, error) {

	defer Instrument("VFSListDirectory")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view the VFS.")
	}

	vfs_service, err := services.GetVFSService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, err := vfs_service.ListDirectories(ctx,
		org_config_obj, in.ClientId, in.VfsComponents)
	return result, err
}

func (self *ApiServer) VFSStatDirectory(
	ctx context.Context,
	in *api_proto.VFSListRequest) (*api_proto.VFSListResponse, error) {

	defer Instrument("VFSStatDirectory")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to launch flows.")
	}

	vfs_service, err := services.GetVFSService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, err := vfs_service.StatDirectory(
		org_config_obj, in.ClientId, in.VfsComponents)
	return result, Status(self.verbose, err)
}

func (self *ApiServer) VFSStatDownload(
	ctx context.Context,
	in *api_proto.VFSStatDownloadRequest) (*flows_proto.VFSDownloadInfo, error) {

	defer Instrument("VFSStatDownload")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view the VFS.")
	}

	vfs_service, err := services.GetVFSService(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result, err := vfs_service.StatDownload(
		org_config_obj, in.ClientId, in.Accessor, in.Components)
	return result, Status(self.verbose, err)
}

func (self *ApiServer) VFSRefreshDirectory(
	ctx context.Context,
	in *api_proto.VFSRefreshDirectoryRequest) (
	*flows_proto.ArtifactCollectorResponse, error) {

	defer Instrument("VFSRefreshDirectory")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.COLLECT_CLIENT
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to launch flows.")
	}

	result, err := vfsRefreshDirectory(
		self, ctx, in.ClientId, in.VfsComponents, in.Depth)
	return result, Status(self.verbose, err)
}

func (self *ApiServer) VFSGetBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.VFSFileBuffer, error) {

	defer Instrument("VFSGetBuffer")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	// The user may request to download a buffer from any org.
	if !utils.CompareOrgIds(org_config_obj.OrgId, in.OrgId) {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		org_config_obj, err = org_manager.GetOrgConfig(in.OrgId)
		if err != nil {
			return nil, Status(self.verbose, err)
		}
	}

	principal := user_record.Name

	// Make sure the principal has permission in the org.
	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
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
		pathspec = path_specs.FromGenericComponentList(in.Components)

	} else {
		return nil, status.Error(codes.InvalidArgument,
			"Invalid pathspec")
	}

	padding := true
	if in.Padding != nil {
		padding = *in.Padding
	}

	result, err := vfsGetBuffer(org_config_obj, in.ClientId, pathspec, in.Offset, in.Length, padding)

	return result, Status(self.verbose, err)
}

func (self *ApiServer) GetTable(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {

	defer Instrument("GetTable")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view results.")
	}

	result, err := tables.GetTable(ctx, org_config_obj, in, principal)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return result, nil
}

func (self *ApiServer) GetArtifacts(
	ctx context.Context,
	in *api_proto.GetArtifactsRequest) (
	*artifacts_proto.ArtifactDescriptors, error) {

	defer Instrument("GetArtifacts")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view custom artifacts.")
	}

	if len(in.Names) > 0 {
		result := &artifacts_proto.ArtifactDescriptors{}
		manager, err := services.GetRepositoryManager(org_config_obj)
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		repository, err := manager.GetGlobalRepository(org_config_obj)
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		for _, name := range in.Names {
			artifact, pres := repository.Get(ctx, org_config_obj, name)
			if !pres {
				continue
			}

			artifact_clone := proto.Clone(artifact).(*artifacts_proto.Artifact)
			for _, s := range artifact_clone.Sources {
				s.Queries = nil
			}
			artifact_clone.Raw = ""

			if pres {
				result.Items = append(result.Items, artifact_clone)
			}
		}
		return result, nil
	}

	if in.ReportType != "" {
		return getReportArtifacts(
			ctx, org_config_obj, in.ReportType, in.NumberOfResults)
	}

	result, err := searchArtifact(
		ctx, org_config_obj, in.SearchTerm,
		in.Type, in.NumberOfResults, in.Fields)
	return result, Status(self.verbose, err)
}

func (self *ApiServer) GetArtifactFile(
	ctx context.Context,
	in *api_proto.GetArtifactRequest) (
	*api_proto.GetArtifactResponse, error) {

	defer Instrument("GetArtifactFile")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			"User is not allowed to view custom artifacts.")
	}

	artifact, err := getArtifactFile(ctx, org_config_obj, in.Name)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := &api_proto.GetArtifactResponse{
		Artifact: artifact,
	}
	return result, nil
}

func (self *ApiServer) SetArtifactFile(
	ctx context.Context,
	in *api_proto.SetArtifactRequest) (*api_proto.SetArtifactResponse, error) {

	defer Instrument("SetArtifactFile")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.ARTIFACT_WRITER

	// Verify the artifact first, then only set it if there are no
	// errors or warnings.
	if in.Op == api_proto.SetArtifactRequest_CHECK_AND_SET {
		state, err := checkArtifact(ctx, org_config_obj, in.Artifact)
		if err != nil {
			return nil, Status(self.verbose, err)
		}

		// report the errors and warnings
		if len(state.Errors) != 0 || len(state.Warnings) != 0 {
			res := &api_proto.SetArtifactResponse{
				Error:    true,
				Warnings: state.Warnings,
			}

			for _, e := range state.Errors {
				res.Errors = append(res.Errors, e)
			}

			return res, nil
		}

		// Fallback to regular setting.
		in.Op = api_proto.SetArtifactRequest_SET
	}

	// We need to load the artifact to figure out what type it is
	// first. Depending on the artifact type we need to check the
	// relevant permission.
	manager, err := services.GetRepositoryManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	tmp_repository := manager.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(
		in.Artifact, services.ArtifactOptions{
			ValidateArtifact: true,
		})
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	switch strings.ToUpper(artifact_definition.Type) {
	case "CLIENT", "CLIENT_EVENT":
		permissions = acls.ARTIFACT_WRITER
	case "SERVER", "SERVER_EVENT", "NOTEBOOK":
		permissions = acls.SERVER_ARTIFACT_WRITER
	}

	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to modify artifacts (%v).", permissions))
	}

	definition, err := setArtifactFile(ctx, org_config_obj, principal, in, "")
	if err != nil {
		message := &api_proto.SetArtifactResponse{
			Error:        true,
			ErrorMessage: fmt.Sprintf("%v", err),
		}
		return message, Status(self.verbose, errors.New(message.ErrorMessage))
	}

	err = services.LogAudit(ctx,
		org_config_obj, principal, "SetArtifactFile",
		ordereddict.NewDict().
			Set("artifact", definition.Name).
			Set("details", in.Artifact))

	return &api_proto.SetArtifactResponse{}, err
}

func (self *ApiServer) Query(
	in *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer) error {

	defer Instrument("Query")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(stream.Context())
	if err != nil {
		return err
	}
	principal := user_record.Name

	// If the caller wants to switch orgs, change the config to point
	// to that org. We check permission immediately below to ensure
	// they actually have the permission to query this org.
	if in.OrgId != "" {
		// Fetch the appropriate config file fro the org manager.
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return Status(self.verbose, err)
		}

		org_config_obj, err = org_manager.GetOrgConfig(in.OrgId)
		if err != nil {
			return Status(self.verbose, err)
		}
	}

	// Check that the principal is allowed to issue queries.
	permissions := acls.ANY_QUERY
	ok, err := services.CheckAccess(org_config_obj, principal, permissions)
	if err != nil {
		return status.Error(codes.PermissionDenied,
			fmt.Sprintf("User %v is not allowed to run queries.",
				principal))
	}

	if !ok {
		return status.Error(codes.PermissionDenied, fmt.Sprintf(
			"Permission denied: User %v requires permission %v to run queries",
			principal, permissions))
	}

	return streamQuery(stream.Context(), org_config_obj, in, stream, principal)
}

func (self *ApiServer) GetServerMonitoringState(
	ctx context.Context,
	in *emptypb.Empty) (
	*flows_proto.ArtifactCollectorArgs, error) {

	defer Instrument("GetServerMonitoringState")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to read results (%v).", permissions))
	}

	server_event_manager, err := services.GetServerEventManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return server_event_manager.Get(), nil
}

func (self *ApiServer) SetServerMonitoringState(
	ctx context.Context,
	in *flows_proto.ArtifactCollectorArgs) (
	*flows_proto.ArtifactCollectorArgs, error) {

	defer Instrument("SetServerMonitoringState")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	// Monitoring queries needs same permissions as regular artifact
	// collections.
	permissions := acls.COLLECT_SERVER
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to modify artifacts (%v).", permissions))
	}

	server_event_manager, err := services.GetServerEventManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = server_event_manager.Update(ctx, org_config_obj, principal, in)
	return in, Status(self.verbose, err)
}

func (self *ApiServer) GetClientMonitoringState(
	ctx context.Context, in *flows_proto.GetClientMonitoringStateRequest) (
	*flows_proto.ClientEventTable, error) {

	defer Instrument("GetClientMonitoringState")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.READ_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to read monitoring artifacts (%v).", permissions))
	}

	manager, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	result := manager.GetClientMonitoringState()
	if in.ClientId != "" {
		message := manager.GetClientUpdateEventTableMessage(
			ctx, org_config_obj, in.ClientId)
		result.ClientMessage = message
	}

	return result, Status(self.verbose, err)
}

func (self *ApiServer) SetClientMonitoringState(
	ctx context.Context,
	in *flows_proto.ClientEventTable) (
	*emptypb.Empty, error) {

	defer Instrument("SetClientMonitoringState")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name
	permissions := acls.COLLECT_CLIENT
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to modify monitoring artifacts (%v).", permissions))
	}

	manager, err := services.ClientEventManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	err = manager.SetClientMonitoringState(ctx, org_config_obj, principal, in)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	return &emptypb.Empty{}, nil
}

func (self *ApiServer) CreateDownloadFile(ctx context.Context,
	in *api_proto.CreateDownloadRequest) (*api_proto.CreateDownloadResponse, error) {

	defer Instrument("CreateDownloadFile")()

	users := services.GetUserManager()
	user_record, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}
	principal := user_record.Name

	permissions := acls.PREPARE_RESULTS
	perm, err := services.CheckAccess(org_config_obj, principal, permissions)
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to create downloads (%v).", permissions))
	}

	// Log an audit event.
	err = services.LogAudit(ctx,
		org_config_obj, principal, "CreateDownloadRequest",
		ordereddict.NewDict().Set("request", in))
	if !perm || err != nil {
		return nil, PermissionDenied(err,
			fmt.Sprintf("User is not allowed to create downloads (%v).", permissions))
	}

	format := ""
	if in.JsonFormat && !in.CsvFormat {
		format = "json"
	} else if in.CsvFormat && !in.JsonFormat {
		format = "csv_only"
	} else if in.CsvFormat && in.JsonFormat {
		format = "csv"
	} else {
		format = "json"
	}

	query := ""
	env := ordereddict.NewDict()
	if in.FlowId != "" && in.ClientId != "" {
		query = `SELECT create_flow_download(password=Password, format=Format,
      expand_sparse=ExpandSparse, client_id=ClientId, flow_id=FlowId) AS VFSPath
      FROM scope()`

		env.Set("ClientId", in.ClientId).
			Set("FlowId", in.FlowId).
			Set("Password", in.Password).
			Set("Format", format).
			Set("ExpandSparse", in.ExpandSparse)

	} else if in.HuntId != "" {
		query = `SELECT create_hunt_download(password=Password,
      expand_sparse=ExpandSparse,
      hunt_id=HuntId, only_combined=OnlyCombined, format=Format) AS VFSPath
      FROM scope()`

		env.Set("HuntId", in.HuntId).
			Set("Format", format).
			Set("Password", in.Password).
			Set("OnlyCombined", in.OnlyCombinedHunt).
			Set("ExpandSparse", in.ExpandSparse)
	}

	manager, err := services.GetRepositoryManager(org_config_obj)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	scope := manager.BuildScope(
		services.ScopeBuilder{
			Config:     org_config_obj,
			Env:        env,
			ACLManager: acl_managers.NewServerACLManager(org_config_obj, principal),
			Logger:     logging.NewPlainLogger(org_config_obj, &logging.FrontendComponent),
		})
	defer scope.Close()

	vql, err := vfilter.Parse(query)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	sub_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	result := &api_proto.CreateDownloadResponse{}
	for row := range vql.Eval(sub_ctx, scope) {
		result.VfsPath = vql_subsystem.GetStringFromRow(scope, row, "VFSPath")
	}

	return result, Status(self.verbose, err)
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
		return errors.Wrap(err, 0)
	}

	// Use the server certificate to secure the gRPC connection.
	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return errors.Wrap(err, 0)
	}

	// Authenticate API clients using certificates.
	CA_Pool := x509.NewCertPool()
	if config_obj.Client != nil {
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))
	}

	// Create the TLS credentials
	tls_config := &tls.Config{}
	err = getTLSConfig(config_obj, tls_config)
	if err != nil {
		return err
	}

	// Only accept certs signed by the Velociraptor internal CA
	tls_config.ClientAuth = tls.RequireAndVerifyClientCert
	tls_config.Certificates = []tls.Certificate{cert}
	tls_config.ClientCAs = CA_Pool

	creds := credentials.NewTLS(tls_config)

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	api_proto.RegisterAPIServer(
		grpcServer,
		&ApiServer{
			server_obj:         server_obj,
			verbose:            config_obj.Verbose,
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

	mux := api_utils.NewServeMux()
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
		timeout_ctx, cancel := utils.WithTimeoutCause(
			context.Background(), 10*time.Second,
			errors.New("Monitoring Service deadline reached"))
		defer cancel()

		err := server.Shutdown(timeout_ctx)
		if err != nil {
			logger.Error("Prometheus shutdown error: %v", err)
		}
	}()

	logger.Info("Launched Prometheus monitoring server on %v ", bind_addr)
	return nil
}
