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
// Client stubs for GRPC connections.
package grpc_client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync"
	"time"

	grpcpool "github.com/Velocidex/grpc-go-pool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

// The different types of caller supported. This affects the user
// identity we use when making the API call.
type CallerIdentity int

const (
	// Used by the gateway proxy to call the API on behalf of the
	// GUI. This identity causes the real user identity to be
	// recovered from the embedded metadata.
	GRPC_GW CallerIdentity = iota + 1

	// Used by user API calls
	API_User

	// Used by the Velociraptor Server to make minion to master API
	// calls. Implicitely trusted.
	SuperUser
)

var (
	Factory APIClientFactory = &DummyGRPCAPIClient{}

	grpcCallCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "grpc_client_calls",
		Help: "Total number of internal gRPC calls.",
	})

	grpcTimeoutCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "grpc_client_timeouts",
		Help: "Total number of timeouts in getting a connection from the pool.",
	})

	grpcPoolWaiters = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "grpc_client_waiters",
		Help: "Total number of waiters for a grpc client channel.",
	})
)

type gRPCPool struct {
	// Cache the creds for internal gRPC connections.
	mu    sync.Mutex
	creds credentials.TransportCredentials

	pool    *grpcpool.Pool
	address string

	config_obj *config_proto.Config
}

// There are three types of pools reserved for different caller
// identities. Keeping the pools separated ensures we can never call
// the API with the wrong identities. The server assigns different
// permissions to the different identities.
func NewGRPCPool(config_obj *config_proto.Config,
	identity CallerIdentity) (*gRPCPool, error) {
	var certificate, private_key, ca_certificate string

	// Expect the server present the correct server certificate. This
	// pins the acceptable server certificate to ensure we can not
	// connect to the wrong server.
	server_name := utils.GetSuperuserName(config_obj)

	// Configure the credentials based on which identity is required.
	switch identity {
	case SuperUser:
		if config_obj.Frontend != nil && config_obj.Client != nil {
			// Present the frontend certificate as our identity. This
			// will be implicitely trusted for every ACL.
			certificate = config_obj.Frontend.Certificate
			private_key = config_obj.Frontend.PrivateKey
			ca_certificate = config_obj.Client.CaCertificate
		}

	case API_User:
		if config_obj.ApiConfig != nil &&
			config_obj.ApiConfig.ClientCert != "" {
			// For an API connection, present the API certificate to
			// connect with.
			certificate = config_obj.ApiConfig.ClientCert
			private_key = config_obj.ApiConfig.ClientPrivateKey
			ca_certificate = config_obj.ApiConfig.CaCertificate
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
		}
	}

	// Identity not configured - This is not really an error but we
	// wont be able to make calls using this identity.
	if certificate == "" {
		return nil, nil
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

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      CA_Pool,
		ServerName:   server_name,
	})

	// Do not create the pool until first call
	return &gRPCPool{
		creds:      creds,
		address:    GetAPIConnectionString(config_obj),
		config_obj: config_obj,
	}, nil
}

type APIClientFactory interface {
	GetAPIClient(
		ctx context.Context,
		identity CallerIdentity,
		config_obj *config_proto.Config) (
		api_proto.APIClient, func() error, error)
}

// Maintain separate pools for different identities.
type GRPCAPIClient struct {
	GRPC_GW, API_User, SuperUser *gRPCPool
}

func NewGRPCAPIClient(config_obj *config_proto.Config) (*GRPCAPIClient, error) {
	res := &GRPCAPIClient{}
	var err error

	res.GRPC_GW, err = NewGRPCPool(config_obj, GRPC_GW)
	if err != nil {
		return nil, err
	}
	res.API_User, err = NewGRPCPool(config_obj, API_User)
	if err != nil {
		return nil, err
	}

	res.SuperUser, err = NewGRPCPool(config_obj, SuperUser)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (self GRPCAPIClient) GetAPIClient(
	ctx context.Context, identity CallerIdentity,
	config_obj *config_proto.Config) (
	api_proto.APIClient, func() error, error) {

	var pool *gRPCPool
	switch identity {
	case GRPC_GW:
		pool = self.GRPC_GW
		if pool == nil {
			return nil, nil, errors.New(
				"No gateway identity configured (GUI.gw_certificate)")
		}
	case API_User:
		pool = self.API_User
		if pool == nil {
			return nil, nil, errors.New(
				"No API user identity configured (API.client_cert)")
		}
	case SuperUser:
		pool = self.SuperUser
		if pool == nil {
			return nil, nil, errors.New(
				"No server identity configured (Frontend.certificate)")
		}
	}

	channel, err := pool.getChannel(ctx)
	if err != nil {
		return nil, nil, err
	}

	grpcCallCounter.Inc()

	return api_proto.NewAPIClient(channel.ClientConn), channel.Close, err
}

func (self *gRPCPool) getChannel(ctx context.Context) (*grpcpool.ClientConn, error) {

	// Collect number of callers waiting for a channel - this
	// indicates backpressure from the grpc pool.
	grpcPoolWaiters.Inc()
	defer grpcPoolWaiters.Dec()

	// Make sure pool is initialized.
	err := self.EnsureInit(ctx, false /* recreate */)
	if err != nil {
		return nil, err
	}

	for {
		conn, err := self.pool.Get(ctx)
		if err == grpcpool.ErrTimeout {
			grpcTimeoutCounter.Inc()
			time.Sleep(time.Second)

			// Try to force a new connection pool in case the master
			// changed it's DNS mapping.
			err := self.EnsureInit(ctx, true /* recreate */)
			if err != nil {
				return nil, err
			}
			continue
		}

		return conn, err
	}
}

// Figure out the correct API connection string from the config
func GetAPIConnectionString(config_obj *config_proto.Config) string {
	if config_obj.ApiConfig != nil && config_obj.ApiConfig.ApiConnectionString != "" {
		return config_obj.ApiConfig.ApiConnectionString
	}

	if config_obj.API == nil {
		return ""
	}

	switch config_obj.API.BindScheme {
	case "tcp":
		hostname := config_obj.API.Hostname
		if config_obj.API.BindAddress == "127.0.0.1" {
			hostname = config_obj.API.BindAddress
		}
		if hostname == "" {
			hostname = config_obj.API.BindAddress
		}
		return fmt.Sprintf("%s:%d", hostname, config_obj.API.BindPort)

	case "unix":
		return fmt.Sprintf("unix://%s", config_obj.API.BindAddress)
	}

	panic("Unknown API.BindScheme")
}

// Make sure the pool is established and running.
func (self *gRPCPool) EnsureInit(
	ctx context.Context, recreate bool) (err error) {

	self.mu.Lock()
	defer self.mu.Unlock()

	if !recreate && self.pool != nil {
		return nil
	}

	// Build a new pool.
	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(self.creds),
		}

		if self.config_obj.ApiConfig != nil &&
			self.config_obj.ApiConfig.MaxGrpcRecvSize > 0 {
			opts = append(opts,
				grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(
					int(self.config_obj.ApiConfig.MaxGrpcRecvSize))))

			logger := logging.GetLogger(self.config_obj, &logging.GUIComponent)
			logger.Info("<green>API Client</>: Limiting gRPC message size to %v",
				self.config_obj.ApiConfig.MaxGrpcRecvSize)

		}
		return grpc.DialContext(ctx, self.address, opts...)
	}

	max_size := 100
	max_wait := 60
	if self.config_obj.Frontend != nil {
		if self.config_obj.Frontend.GRPCPoolMaxSize > 0 {
			max_size = int(self.config_obj.Frontend.GRPCPoolMaxSize)
		}

		if self.config_obj.Frontend.GRPCPoolMaxWait > 0 {
			max_wait = int(self.config_obj.Frontend.GRPCPoolMaxWait)
		}
	}

	self.pool, err = grpcpool.NewWithContext(ctx,
		factory, 1, max_size, time.Duration(max_wait)*time.Second)
	if err != nil {
		return fmt.Errorf(
			"Unable to connect to gRPC server: %v: %v", self.address, err)
	}
	return nil
}
