package api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self *ApiServer) redirectCall(
	ctx context.Context,
	req interface{},
	method string,
	org_record *api_proto.OrgRecord,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	users := services.GetUserManager()
	_, org_config_obj, err := users.GetUserFromContext(ctx)
	if err != nil {
		return nil, Status(self.verbose, err)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, utils.UnknownError
	}

	user := md.Get(constants.GRPC_MD_USER)
	if len(user) != 1 {
		return nil, utils.UnknownError
	}

	// Relay the real username through the metadata
	new_md := metadata.New(map[string]string{
		constants.GRPC_MD_USER:   user[0],
		constants.GRPC_MD_ORG_ID: org_record.RedirectedOrgId,
	})

	call_ctx := metadata.NewOutgoingContext(ctx, new_md)

	api_client, closer, err := self.api_client_factory.GetAPIClientForOrgRedirect(
		ctx, org_config_obj)
	if err != nil {
		return nil, err
	}
	defer closer()

	// Relay all the methods to the remote end.
	switch method {
	case "/proto.API/CreateHunt":
		r, ok := req.(*api_proto.Hunt)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CreateHunt(call_ctx, r)

	case "/proto.API/EstimateHunt":
		r, ok := req.(*api_proto.HuntEstimateRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.EstimateHunt(call_ctx, r)

	case "/proto.API/GetHuntTable":
		r, ok := req.(*api_proto.GetTableRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetHuntTable(call_ctx, r)

	case "/proto.API/ListHunts":
		r, ok := req.(*api_proto.ListHuntsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ListHunts(call_ctx, r)

	case "/proto.API/GetHunt":
		r, ok := req.(*api_proto.GetHuntRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetHunt(call_ctx, r)

	case "/proto.API/GetHuntTags":
		r, ok := req.(*empty.Empty)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetHuntTags(call_ctx, r)

	case "/proto.API/ModifyHunt":
		r, ok := req.(*api_proto.HuntMutation)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ModifyHunt(call_ctx, r)

	case "/proto.API/GetHuntFlows":
		r, ok := req.(*api_proto.GetTableRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetHuntFlows(call_ctx, r)

	case "/proto.API/GetHuntResults":
		r, ok := req.(*api_proto.GetHuntResultsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetHuntResults(call_ctx, r)

	case "/proto.API/NotifyClients":
		r, ok := req.(*api_proto.NotificationRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.NotifyClients(call_ctx, r)

	case "/proto.API/LabelClients":
		r, ok := req.(*api_proto.LabelClientsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.LabelClients(call_ctx, r)

	case "/proto.API/ListClients":
		r, ok := req.(*api_proto.SearchClientsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ListClients(call_ctx, r)

	case "/proto.API/GetClient":
		r, ok := req.(*api_proto.GetClientRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetClient(call_ctx, r)

	case "/proto.API/GetClientMetadata":
		r, ok := req.(*api_proto.GetClientRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetClientMetadata(call_ctx, r)

	case "/proto.API/SetClientMetadata":
		r, ok := req.(*api_proto.SetClientMetadataRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SetClientMetadata(call_ctx, r)

	case "/proto.API/GetClientFlows":
		r, ok := req.(*api_proto.GetTableRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetClientFlows(call_ctx, r)

		// User management is done by the local server. This includes
		// global user preferences, like themes, language settings
		// etc.
	case "/proto.API/GetUserUITraits",
		"/proto.API/SetGUIOptions",
		"/proto.API/GetUsers",
		"/proto.API/GetGlobalUsers",
		"/proto.API/GetUserRoles",
		"/proto.API/SetUserRoles",
		"/proto.API/GetUser",
		"/proto.API/CreateUser":
		return handler(ctx, req)

		// Favorites are per org since they need to refer to an org's
		// artifacts.
	case "/proto.API/GetUserFavorites":
		r, ok := req.(*api_proto.Favorite)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetUserFavorites(call_ctx, r)

	case "/proto.API/VFSListDirectory":
		r, ok := req.(*api_proto.VFSListRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSListDirectory(call_ctx, r)

	case "/proto.API/VFSListDirectoryFiles":
		r, ok := req.(*api_proto.GetTableRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSListDirectoryFiles(call_ctx, r)

	case "/proto.API/VFSRefreshDirectory":
		r, ok := req.(*api_proto.VFSRefreshDirectoryRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSRefreshDirectory(call_ctx, r)

	case "/proto.API/VFSStatDirectory":
		r, ok := req.(*api_proto.VFSListRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSStatDirectory(call_ctx, r)

	case "/proto.API/VFSStatDownload":
		r, ok := req.(*api_proto.VFSStatDownloadRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSStatDownload(call_ctx, r)

	case "/proto.API/VFSDownloadFile":
		r, ok := req.(*api_proto.VFSStatDownloadRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.VFSDownloadFile(call_ctx, r)

	case "/proto.API/GetTable":
		r, ok := req.(*api_proto.GetTableRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetTable(call_ctx, r)

	case "/proto.API/SearchFile":
		r, ok := req.(*api_proto.SearchFileRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SearchFile(call_ctx, r)

	case "/proto.API/CollectArtifact":
		r, ok := req.(*flows_proto.ArtifactCollectorArgs)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CollectArtifact(call_ctx, r)

	case "/proto.API/CancelFlow":
		r, ok := req.(*api_proto.ApiFlowRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CancelFlow(call_ctx, r)

	case "/proto.API/ResumeFlow":
		r, ok := req.(*api_proto.ApiFlowRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ResumeFlow(call_ctx, r)

	case "/proto.API/GetFlowDetails":
		r, ok := req.(*api_proto.ApiFlowRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetFlowDetails(call_ctx, r)

	case "/proto.API/GetFlowRequests":
		r, ok := req.(*api_proto.ApiFlowRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetFlowRequests(call_ctx, r)

	case "/proto.API/GetKeywordCompletions":
		r, ok := req.(*empty.Empty)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetKeywordCompletions(call_ctx, r)

	case "/proto.API/ReformatVQL":
		r, ok := req.(*api_proto.ReformatVQLMessage)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ReformatVQL(call_ctx, r)

	case "/proto.API/GetArtifacts":
		r, ok := req.(*api_proto.GetArtifactsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetArtifacts(call_ctx, r)

	case "/proto.API/GetArtifactFile":
		r, ok := req.(*api_proto.GetArtifactRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetArtifactFile(call_ctx, r)

	case "/proto.API/SetArtifactFile":
		r, ok := req.(*api_proto.SetArtifactRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SetArtifactFile(call_ctx, r)

	case "/proto.API/LoadArtifactPack":
		r, ok := req.(*api_proto.LoadArtifactPackRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.LoadArtifactPack(call_ctx, r)

	case "/proto.API/SearchDocs":
		r, ok := req.(*api_proto.DocSearchRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SearchDocs(call_ctx, r)

	case "/proto.API/GetToolInfo":
		r, ok := req.(*artifacts_proto.Tool)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetToolInfo(call_ctx, r)

	case "/proto.API/SetToolInfo":
		r, ok := req.(*artifacts_proto.Tool)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SetToolInfo(call_ctx, r)

	case "/proto.API/GetReport":
		r, ok := req.(*api_proto.GetReportRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetReport(call_ctx, r)

	case "/proto.API/GetServerMonitoringState":
		r, ok := req.(*empty.Empty)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetServerMonitoringState(call_ctx, r)

	case "/proto.API/SetServerMonitoringState":
		r, ok := req.(*flows_proto.ArtifactCollectorArgs)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.SetServerMonitoringState(call_ctx, r)

	case "/proto.API/ListAvailableEventResults":
		r, ok := req.(*api_proto.ListAvailableEventResultsRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ListAvailableEventResults(call_ctx, r)

	case "/proto.API/CreateDownloadFile":
		r, ok := req.(*api_proto.CreateDownloadRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CreateDownloadFile(call_ctx, r)

	case "/proto.API/GetNotebooks":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetNotebooks(call_ctx, r)

	case "/proto.API/NewNotebook":
		r, ok := req.(*api_proto.NotebookMetadata)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.NewNotebook(call_ctx, r)

	case "/proto.API/UpdateNotebook":
		r, ok := req.(*api_proto.NotebookMetadata)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.UpdateNotebook(call_ctx, r)

	case "/proto.API/DeleteNotebook":
		r, ok := req.(*api_proto.NotebookMetadata)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.DeleteNotebook(call_ctx, r)

	case "/proto.API/NewNotebookCell":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.NewNotebookCell(call_ctx, r)

	case "/proto.API/GetNotebookCell":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetNotebookCell(call_ctx, r)

	case "/proto.API/UpdateNotebookCell":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.UpdateNotebookCell(call_ctx, r)

	case "/proto.API/RevertNotebookCell":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.RevertNotebookCell(call_ctx, r)

	case "/proto.API/CancelNotebookCell":
		r, ok := req.(*api_proto.NotebookCellRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CancelNotebookCell(call_ctx, r)

	case "/proto.API/CreateNotebookDownloadFile":
		r, ok := req.(*api_proto.NotebookExportRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.CreateNotebookDownloadFile(call_ctx, r)

	case "/proto.API/UploadNotebookAttachment":
		r, ok := req.(*api_proto.NotebookFileUploadRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.UploadNotebookAttachment(call_ctx, r)

	case "/proto.API/RemoveNotebookAttachment":
		r, ok := req.(*api_proto.NotebookFileUploadRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.RemoveNotebookAttachment(call_ctx, r)

	case "/proto.API/AnnotateTimeline":
		r, ok := req.(*api_proto.AnnotationRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.AnnotateTimeline(call_ctx, r)

	case "/proto.API/GetSecretDefinitions":
		r, ok := req.(*empty.Empty)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetSecretDefinitions(call_ctx, r)

	case "/proto.API/AddSecret":
		r, ok := req.(*api_proto.Secret)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.AddSecret(call_ctx, r)

	case "/proto.API/ModifySecret":
		r, ok := req.(*api_proto.ModifySecretRequest)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.ModifySecret(call_ctx, r)

	case "/proto.API/GetSecret":
		r, ok := req.(*api_proto.Secret)
		if !ok {
			return nil, utils.InvalidArgError
		}
		return api_client.GetSecret(call_ctx, r)
	}

	return nil, utils.Wrap(utils.NotImplementedError, "Unable to forward call %#v", method)
}

// This middleware runs just before the gRPC handler runs and after
// the http request was authenticated.
func ProxyInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "metadata is missing")
	}

	org_id := md.Get(constants.GRPC_MD_ORG_ID)
	if len(org_id) == 1 {
		org_manager, err := services.GetOrgManager()
		if err != nil {
			return nil, err
		}

		org_record, err := org_manager.GetOrg(org_id[0])
		if err != nil {
			return nil, err
		}

		if org_record.Redirected {
			server, ok := info.Server.(*ApiServer)
			if !ok {
				return nil, Status(false, utils.UnknownError)
			}

			return server.redirectCall(
				ctx, req, info.FullMethod, org_record, handler)
		}
	}

	// Just relay it normally.
	return handler(ctx, req)
}
