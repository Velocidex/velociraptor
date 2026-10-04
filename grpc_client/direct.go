package grpc_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type DirectConnectionClient struct {
	mu      sync.Mutex
	clients map[CallerIdentity]*grpc.ClientConn
}

func NewDirectConnectionClient() *DirectConnectionClient {
	return &DirectConnectionClient{
		clients: make(map[CallerIdentity]*grpc.ClientConn),
	}
}

func getCreds(
	config_obj *config_proto.Config,
	identity CallerIdentity) (credentials.TransportCredentials, error) {

	var certificate, private_key, ca_certificate string

	// Configure the credentials based on which identity is required.
	switch identity {
	case SuperUser:
		if config_obj.Frontend != nil && config_obj.Client != nil {
			// Present the frontend certificate as our identity. This
			// will be implicitly trusted for every ACL.
			certificate = config_obj.Frontend.Certificate
			private_key = config_obj.Frontend.PrivateKey
			ca_certificate = config_obj.Client.CaCertificate

			if certificate == "" {
				return nil, errors.New(
					"No gateway identity configured (GUI.gw_certificate)")
			}
		}

	case API_User:
		if config_obj.ApiConfig != nil &&
			config_obj.ApiConfig.ClientCert != "" {
			// For an API connection, present the API certificate to
			// connect with.
			certificate = config_obj.ApiConfig.ClientCert
			private_key = config_obj.ApiConfig.ClientPrivateKey
			ca_certificate = config_obj.ApiConfig.CaCertificate
			if certificate == "" {
				return nil, errors.New(
					"No API user identity configured (API.client_cert)")
			}
		}

	case GRPC_GW:
		if config_obj.GUI != nil &&
			config_obj.GUI.GwCertificate != "" &&
			config_obj.Client != nil {
			// For an API connection, present the API certificate to
			// connect with.
			certificate = config_obj.GUI.GwCertificate
			private_key = config_obj.GUI.GwPrivateKey
			ca_certificate = config_obj.Client.CaCertificate
			if certificate == "" {
				return nil, errors.New(
					"No server identity configured (Frontend.certificate)")
			}
		}
	}

	// Identity not configured - This is not really an error but we
	// wont be able to make calls using this identity.
	if certificate == "" {
		return nil, utils.Wrap(utils.InvalidConfigError,
			"No certificate for identity %v", identity)
	}

	// We use the Frontend's certificate because this connection
	// represents an internal connection.
	cert, err := tls.X509KeyPair(
		[]byte(certificate),
		[]byte(private_key))
	if err != nil {
		// This is a critical error - the certs are broken
		return nil, err
	}

	// The server cert must be signed by our CA.
	CA_Pool := x509.NewCertPool()
	CA_Pool.AppendCertsFromPEM([]byte(ca_certificate))

	// Expect the server to present the correct server
	// certificate. This pins the acceptable server certificate to
	// ensure we can not connect to the wrong server.
	server_name := utils.GetSuperuserName(config_obj)

	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      CA_Pool,
		// Only allow connections to this server name
		ServerName: server_name,
	}), nil

}

func (self *DirectConnectionClient) NewClientConn(
	ctx context.Context,
	config_obj *config_proto.Config,
	identity CallerIdentity,
) (*grpc.ClientConn, error) {
	creds, err := getCreds(config_obj, identity)
	if err != nil {
		return nil, err
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	if config_obj.ApiConfig != nil &&
		config_obj.ApiConfig.MaxGrpcRecvSize > 0 {
		opts = append(opts,
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(
				int(config_obj.ApiConfig.MaxGrpcRecvSize))))

		logger := logging.GetLogger(config_obj, &logging.GUIComponent)
		logger.Info("<green>API Client</>: Limiting gRPC message size to %v",
			config_obj.ApiConfig.MaxGrpcRecvSize)
	}

	address := GetAPIConnectionString(config_obj)

	return grpc.DialContext(ctx, address, opts...)
}

func (self *DirectConnectionClient) Close() error {
	return nil
}

func (self *DirectConnectionClient) getClientForIdentity(
	ctx context.Context, identity CallerIdentity,
	config_obj *config_proto.Config) (*grpc.ClientConn, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	client, pres := self.clients[identity]
	if pres {
		return client, nil
	}

	new_client, err := self.NewClientConn(ctx, config_obj, identity)
	if err != nil {
		return nil, err
	}

	self.clients[identity] = new_client
	return new_client, nil
}

func (self *DirectConnectionClient) GetAPIClient(
	ctx context.Context, identity CallerIdentity,
	config_obj *config_proto.Config) (
	api_proto.APIClient, func() error, error) {

	conn, err := self.getClientForIdentity(ctx, identity, config_obj)
	if err != nil {
		return nil, nil, err
	}

	// Total gRPC calls made for the life of the process.
	grpcCallCounter.Inc()

	// Keep track of how many stubs are currently active.
	grpcStubs.Inc()
	api_client := api_proto.NewAPIClient(conn)

	return api_client,
		// This is called when the stub is done with.
		func() error {
			grpcStubs.Dec()
			return nil
		}, err
}
