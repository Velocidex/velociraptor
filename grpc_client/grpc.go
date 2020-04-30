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

	grpcpool "github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	// Cache the creds for internal gRPC connections.
	mu    sync.Mutex
	creds credentials.TransportCredentials

	pool_mu sync.Mutex
	pool    *grpcpool.Pool
	address string

	Factory APIClientFactory = GRPCAPIClient{}
)

func getCreds(config_obj *config_proto.Config) credentials.TransportCredentials {
	mu.Lock()
	defer mu.Unlock()

	if creds == nil {
		certificate := config_obj.Frontend.Certificate
		private_key := config_obj.Frontend.PrivateKey
		ca_certificate := config_obj.Client.CaCertificate

		if config_obj.ApiConfig.ClientCert != "" {
			certificate = config_obj.ApiConfig.ClientCert
			private_key = config_obj.ApiConfig.ClientPrivateKey
			ca_certificate = config_obj.ApiConfig.CaCertificate
		}

		// We use the Frontend's certificate because this connection
		// represents an internal connection.
		cert, err := tls.X509KeyPair(
			[]byte(certificate),
			[]byte(private_key))
		if err != nil {
			return nil
		}

		// The server cert must be signed by our CA.
		CA_Pool := x509.NewCertPool()
		CA_Pool.AppendCertsFromPEM([]byte(ca_certificate))

		creds = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      CA_Pool,
			ServerName:   config_obj.Client.PinnedServerName,
		})
	}

	return creds
}

type APIClientFactory interface {
	GetAPIClient(ctx context.Context,
		config_obj *config_proto.Config) (api_proto.APIClient, func() error, error)
}

type GRPCAPIClient struct{}

func (self GRPCAPIClient) GetAPIClient(
	ctx context.Context,
	config_obj *config_proto.Config) (
	api_proto.APIClient, func() error, error) {
	channel, err := getChannel(ctx, config_obj)

	return api_proto.NewAPIClient(channel.ClientConn), channel.Close, err
}

func getChannel(
	ctx context.Context,
	config_obj *config_proto.Config) (*grpcpool.ClientConn, error) {
	var err error

	pool_mu.Lock()
	defer pool_mu.Unlock()

	// Pool does not exist - make a new one.
	if pool == nil {
		address = GetAPIConnectionString(config_obj)
		factory := func() (*grpc.ClientConn, error) {
			return grpc.Dial(address, grpc.WithTransportCredentials(
				getCreds(config_obj)))

		}

		max_size := 100
		max_wait := 60
		if config_obj.Frontend != nil {
			if config_obj.Frontend.GRPCPoolMaxSize > 0 {
				max_size = int(config_obj.Frontend.GRPCPoolMaxSize)
			}

			if config_obj.Frontend.GRPCPoolMaxWait > 0 {
				max_size = int(config_obj.Frontend.GRPCPoolMaxWait)
			}
		}

		pool, err = grpcpool.New(factory, 1, max_size, time.Duration(max_wait)*time.Second)
		if err != nil {
			return nil, errors.New(
				fmt.Sprintf("Unable to connect to gRPC server: %v: %v", address, err))
		}
	}

	return pool.Get(ctx)
}

func GetAPIConnectionString(config_obj *config_proto.Config) string {
	if config_obj.ApiConfig != nil && config_obj.ApiConfig.ApiConnectionString != "" {
		return config_obj.ApiConfig.ApiConnectionString
	}

	switch config_obj.API.BindScheme {
	case "tcp":
		hostname := config_obj.API.Hostname
		if hostname == "" {
			hostname = config_obj.API.BindAddress
		}
		return fmt.Sprintf("%s:%d", hostname, config_obj.API.BindPort)

	case "unix":
		return fmt.Sprintf("unix://%s", config_obj.API.BindAddress)
	}

	panic("Unknown API.BindScheme")
}
