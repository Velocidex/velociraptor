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

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/server"
	users "www.velocidex.com/golang/velociraptor/users"
	"www.velocidex.com/golang/vfilter"
)

type ApiServer struct {
	config     *api_proto.Config
	server_obj *server.Server
	ca_pool    *x509.CertPool
}

func (self *ApiServer) CancelFlow(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.StartFlowResponse, error) {
	result := &api_proto.StartFlowResponse{}
	user := GetGRPCUserInfo(ctx).Name

	// Empty users are called internally.
	if user != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, user)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to launch flows.")
		}
	}

	result, err := flows.CancelFlow(self.config, in.ClientId, in.FlowId, user)
	if err != nil {
		return nil, err
	}

	// Log this event as and Audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    user,
			"client":  in.ClientId,
			"flow_id": in.FlowId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CancelFlow")

	return result, nil
}

func (self *ApiServer) GetReport(
	ctx context.Context,
	in *api_proto.GetReportRequest) (*api_proto.GetReportResponse, error) {
	return getReport(ctx, self.config, in)
}

func (self *ApiServer) LaunchFlow(
	ctx context.Context,
	in *flows_proto.FlowRunnerArgs) (*api_proto.StartFlowResponse, error) {
	result := &api_proto.StartFlowResponse{}
	in.Creator = GetGRPCUserInfo(ctx).Name

	// Empty creators are called internally.
	if in.Creator != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, in.Creator)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to launch flows.")
		}
	}

	flow_id, err := flows.StartFlow(self.config, in)
	if err != nil {
		return nil, err
	}
	result.FlowId = *flow_id
	result.RunnerArgs = in

	// Notify the client if it is listenning.
	channel := grpc_client.GetChannel(self.config)
	defer channel.Close()

	client := api_proto.NewAPIClient(channel)
	_, err = client.NotifyClients(ctx, &api_proto.NotificationRequest{
		ClientId: in.ClientId,
	})
	if err != nil {
		fmt.Printf("Cant connect: %v\n", err)
		return nil, err
	}

	// Log this event as and Audit event.
	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"client":  in.ClientId,
			"flow_id": flow_id,
			"details": fmt.Sprintf("%v", in),
		}).Info("LaunchFlow")

	return result, nil
}

func (self *ApiServer) CreateHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*api_proto.StartFlowResponse, error) {

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(ctx).Name
	in.HuntId = flows.GetNewHuntId()

	// Empty creators are called internally.
	if in.Creator != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, in.Creator)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to launch hunts.")
		}
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("CreateHunt")

	result := &api_proto.StartFlowResponse{}
	hunt_id, err := flows.CreateHunt(ctx, self.config, in)
	if err != nil {
		return nil, err
	}

	result.FlowId = *hunt_id

	return result, nil
}

func (self *ApiServer) ModifyHunt(
	ctx context.Context,
	in *api_proto.Hunt) (*empty.Empty, error) {

	// Log this event as an Audit event.
	in.Creator = GetGRPCUserInfo(ctx).Name

	// Empty creators are called internally.
	if in.Creator != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, in.Creator)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to modify hunts.")
		}
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":    in.Creator,
			"hunt_id": in.HuntId,
			"details": fmt.Sprintf("%v", in),
		}).Info("ModifyHunt")

	err := flows.ModifyHunt(self.config, in)
	if err != nil {
		return nil, err
	}

	result := &empty.Empty{}
	return result, nil
}

func (self *ApiServer) ListHunts(
	ctx context.Context,
	in *api_proto.ListHuntsRequest) (*api_proto.ListHuntsResponse, error) {
	result, err := flows.ListHunts(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetHunt(
	ctx context.Context,
	in *api_proto.GetHuntRequest) (*api_proto.Hunt, error) {
	result, err := flows.GetHunt(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetHuntResults(
	ctx context.Context,
	in *api_proto.GetHuntResultsRequest) (*api_proto.GetTableResponse, error) {
	env := vfilter.NewDict().
		Set("HuntID", in.HuntId).
		Set("Artifact", in.Artifact)

	// More than 100 results are not very useful in the GUI -
	// users should just download the csv file for post
	// processing.
	result, err := RunVQL(ctx, self.config, env,
		"SELECT * FROM hunt_results(hunt_id=HuntID, "+
			"artifact=Artifact) LIMIT 100")
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) ListClients(
	ctx context.Context,
	in *api_proto.SearchClientsRequest) (*api_proto.SearchClientsResponse, error) {
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
			api_client, err := GetApiClient(self.config, client_id, false)
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
	if in.NotifyAll {
		self.server_obj.Info("sending notification to everyone")
		self.server_obj.NotificationPool.NotifyAll()
	} else if in.ClientId != "" {
		self.server_obj.Info("sending notification to %s", in.ClientId)
		self.server_obj.NotificationPool.Notify(in.ClientId)
	} else {
		return nil, errors.New("client id should be specified")
	}
	return &empty.Empty{}, nil
}

func (self *ApiServer) LabelClients(
	ctx context.Context,
	in *api_proto.LabelClientsRequest) (*api_proto.APIResponse, error) {

	user_name := GetGRPCUserInfo(ctx).Name
	if user_name != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, user_name)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to manipulate labels.")
		}
	}

	result, err := LabelClients(self.config, in)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (self *ApiServer) GetClient(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApiClient, error) {
	api_client, err := GetApiClient(
		self.config,
		in.ClientId,
		!in.Lightweight, // Detailed
	)
	if err != nil {
		return nil, err
	}

	return api_client, nil
}

func (self *ApiServer) DescribeTypes(
	ctx context.Context,
	in *empty.Empty) (*artifacts_proto.Types, error) {
	return describeTypes(), nil
}

func (self *ApiServer) GetClientFlows(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowResponse, error) {

	// HTTP HEAD requests against this method are used by the GUI
	// for auth checks.
	result := &api_proto.ApiFlowResponse{}
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		method := md.Get("METHOD")
		if len(method) > 0 && method[0] == "HEAD" {
			if IsUserApprovedForClient(self.config, &md, in.ClientId) {
				return result, nil
			}
			return nil, status.New(
				codes.PermissionDenied, "Not authorized").Err()
		}
	}

	result, err := flows.GetFlows(self.config, in.ClientId, in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetClientApprovalForUser(
	ctx context.Context,
	in *api_proto.GetClientRequest) (*api_proto.ApprovalList, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		return getClientApprovalForUser(self.config, &md, in.ClientId), nil
	}
	return nil, status.New(
		codes.PermissionDenied, "Not authorized").Err()
}

func (self *ApiServer) GetUserUITraits(
	ctx context.Context,
	in *empty.Empty) (*api_proto.ApiGrrUser, error) {
	result := NewDefaultUserObject(self.config)
	user_info := GetGRPCUserInfo(ctx)

	result.Username = user_info.Name
	result.InterfaceTraits.Picture = user_info.Picture

	return result, nil
}

func (self *ApiServer) GetFlowDetails(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlow, error) {

	result, err := flows.GetFlowDetails(self.config, in.ClientId, in.FlowId)
	return result, err
}

func (self *ApiServer) GetFlowRequests(
	ctx context.Context,
	in *api_proto.ApiFlowRequest) (*api_proto.ApiFlowRequestDetails, error) {
	result, err := flows.GetFlowRequests(self.config, in.ClientId, in.FlowId,
		in.Offset, in.Count)
	return result, err
}

func (self *ApiServer) GetUserNotifications(
	ctx context.Context,
	in *api_proto.GetUserNotificationsRequest) (
	*api_proto.GetUserNotificationsResponse, error) {
	result, err := users.GetUserNotifications(
		self.config, GetGRPCUserInfo(ctx).Name, in.ClearPending)
	return result, err
}

func (self *ApiServer) GetUserNotificationCount(
	ctx context.Context,
	in *empty.Empty) (*api_proto.UserNotificationCount, error) {
	n, err := users.GetUserNotificationCount(self.config, GetGRPCUserInfo(ctx).Name)
	return &api_proto.UserNotificationCount{Count: n}, err
}

func (self *ApiServer) GetFlowDescriptors(
	ctx context.Context,
	in *empty.Empty) (*api_proto.FlowDescriptors, error) {
	result, err := flows.GetFlowDescriptors()
	return result, err
}

func (self *ApiServer) VFSListDirectory(
	ctx context.Context,
	in *flows_proto.VFSListRequest) (*actions_proto.VQLResponse, error) {
	result, err := vfsListDirectory(
		self.config, in.ClientId, in.VfsPath)
	return result, err
}

func (self *ApiServer) VFSRefreshDirectory(
	ctx context.Context,
	in *api_proto.VFSRefreshDirectoryRequest) (
	*api_proto.StartFlowResponse, error) {

	result, err := vfsRefreshDirectory(
		self, ctx, in.ClientId, in.VfsPath, in.Depth)
	return result, err
}

func (self *ApiServer) VFSGetBuffer(
	ctx context.Context,
	in *api_proto.VFSFileBuffer) (
	*api_proto.VFSFileBuffer, error) {

	result, err := vfsGetBuffer(
		self.config, in.ClientId, in.VfsPath, in.Offset, in.Length)

	return result, err
}

func (self *ApiServer) GetTable(
	ctx context.Context,
	in *api_proto.GetTableRequest) (*api_proto.GetTableResponse, error) {
	result, err := getTable(self.config, in)
	if err != nil {
		return &api_proto.GetTableResponse{}, nil
	}
	return result, nil
}

func (self *ApiServer) GetArtifacts(
	ctx context.Context,
	in *api_proto.GetArtifactsRequest) (
	*artifacts_proto.ArtifactDescriptors, error) {
	result := &artifacts_proto.ArtifactDescriptors{}

	repository, err := artifacts.GetGlobalRepository(self.config)
	if err != nil {
		return nil, err
	}
	for _, name := range repository.List() {
		artifact, pres := repository.Get(name)
		if pres {
			if !in.IncludeEventArtifacts &&
				artifact.Type == "event" {
				continue
			}
			if !in.IncludeServerArtifacts &&
				(artifact.Type == "server" ||
					artifact.Type == "server_event") {
				continue
			}

			result.Items = append(result.Items, artifact)
		}
	}
	return result, nil
}

func (self *ApiServer) GetArtifactFile(
	ctx context.Context,
	in *api_proto.GetArtifactRequest) (
	*api_proto.GetArtifactResponse, error) {

	artifact, err := getArtifactFile(self.config, in.VfsPath)
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

	user_name := GetGRPCUserInfo(ctx).Name
	if user_name != "" {
		// If user is not found then reject it.
		user_record, err := users.GetUser(self.config, user_name)
		if err != nil {
			return nil, err
		}

		if user_record.ReadOnly {
			return nil, errors.New("User is not allowed to modify artifacts.")
		}
	}

	logging.GetLogger(self.config, &logging.Audit).
		WithFields(logrus.Fields{
			"user":          user_name,
			"artifact_file": in.VfsPath,
			"details":       fmt.Sprintf("%v", in.Artifact),
		}).Info("SetArtifactFile")

	err := setArtifactFile(self.config, in.VfsPath, in.Artifact)
	if err != nil {
		return &api_proto.APIResponse{
			Error:        true,
			ErrorMessage: fmt.Sprintf("%v", err),
		}, nil
	}
	return &api_proto.APIResponse{}, nil
}

func (self *ApiServer) Query(
	in *actions_proto.VQLCollectorArgs,
	stream api_proto.API_QueryServer) error {

	// Get the TLS context from the peer and verify its
	// certificate.
	peer, ok := peer.FromContext(stream.Context())
	if !ok {
		return errors.New("cant get peer info")
	}

	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return errors.New("unable to get credentials")
	}

	// Authenticate API clients using certificates.
	for _, peer_cert := range tlsInfo.State.PeerCertificates {
		chains, err := peer_cert.Verify(
			x509.VerifyOptions{Roots: self.ca_pool})
		if err != nil {
			return err
		}

		if len(chains) == 0 {
			return errors.New("no chains verified")
		}

		peer_name := peer_cert.Subject.CommonName

		// Cert is good enough for us, run the query.
		return streamQuery(self.config, in, stream, peer_name)
	}

	return errors.New("no peer certs?")
}

func StartServer(config_obj *api_proto.Config, server_obj *server.Server) error {
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
			config:     config_obj,
			server_obj: server_obj,
			ca_pool:    CA_Pool,
		},
	)
	// Register reflection service.
	reflection.Register(grpcServer)

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Launched gRPC API server on %v ", bind_addr)

	err = grpcServer.Serve(lis)
	if err != nil {
		return err
	}

	return nil
}

func StartMonitoringService(config_obj *api_proto.Config) {
	bind_addr := fmt.Sprintf("%s:%d",
		config_obj.Monitoring.BindAddress,
		config_obj.Monitoring.BindPort)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:    bind_addr,
		Handler: mux,
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			panic("Unable to listen on monitoring")
		}
	}()

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Launched Prometheus monitoring server on %v ", bind_addr)
}
