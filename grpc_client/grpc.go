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
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"

	grpcpool "github.com/Velocidex/grpc-go-pool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

var (
	// Cache the creds for internal gRPC connections.
	mu    sync.Mutex
	creds credentials.TransportCredentials

	pool    *grpcpool.Pool
	address string

	Factory APIClientFactory = GRPCAPIClient{}

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

func getCreds(config_obj *config_proto.Config) (credentials.TransportCredentials, error) {
	if creds == nil {
		var certificate, private_key, ca_certificate, server_name string

		if config_obj.Frontend != nil && config_obj.Client != nil {
			certificate = config_obj.Frontend.Certificate
			private_key = config_obj.Frontend.PrivateKey
			ca_certificate = config_obj.Client.CaCertificate
			server_name = config_obj.Client.PinnedServerName
		}
		if config_obj.ApiConfig != nil &&
			config_obj.ApiConfig.ClientCert != "" {
			certificate = config_obj.ApiConfig.ClientCert
			private_key = config_obj.ApiConfig.ClientPrivateKey
			ca_certificate = config_obj.ApiConfig.CaCertificate
			server_name = config_obj.ApiConfig.PinnedServerName
			if server_name == "" {
				server_name = "VelociraptorServer"
			}
		}

		if certificate == "" {
			return nil, errors.New("Unable to load api certificate")
		}

		// We use the Frontend's certificate because this connection
		// represents an internal connection.
		cert, err := tls.X509KeyPair(
			[]byte(certificate),
			[]byte(private_key))
		if err != nil {
			return nil, err
		}

		// The server cert must be signed by our CA.
		CA_Pool := x509.NewCertPool()
		CA_Pool.AppendCertsFromPEM([]byte(ca_certificate))

		creds = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      CA_Pool,
			ServerName:   server_name,
		})
	}

	return creds, nil
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
	if err != nil {
		return nil, nil, err
	}

	grpcCallCounter.Inc()

	return api_proto.NewAPIClient(channel.ClientConn), channel.Close, err
}

func getChannel(
	ctx context.Context,
	config_obj *config_proto.Config) (*grpcpool.ClientConn, error) {

	// Collect number of callers waiting for a channel - this
	// indicates backpressure from the grpc pool.
	grpcPoolWaiters.Inc()
	defer grpcPoolWaiters.Dec()

	// Make sure pool is initialized.
	err := EnsureInit(ctx, config_obj, false /* recreate */)
	if err != nil {
		return nil, err
	}

	for {
		conn, err := pool.Get(ctx)
		if err == grpcpool.ErrTimeout {
			grpcTimeoutCounter.Inc()
			time.Sleep(time.Second)

			// Try to force a new connection pool in case the master
			// changed it's DNS mapping.
			err := EnsureInit(ctx, config_obj, true /* recreate */)
			if err != nil {
				return nil, err
			}
			continue
		}

		return conn, err
	}
}

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

func EnsureInit(
	ctx context.Context,
	config_obj *config_proto.Config,
	recreate bool) error {

	mu.Lock()
	defer mu.Unlock()

	if !recreate && pool != nil {
		return nil
	}

	address = GetAPIConnectionString(config_obj)
	creds, err := getCreds(config_obj)
	if err != nil {
		return err
	}

	factory := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, address,
			grpc.WithTransportCredentials(creds))
	}

	max_size := 100
	max_wait := 60
	if config_obj.Frontend != nil {
		if config_obj.Frontend.GRPCPoolMaxSize > 0 {
			max_size = int(config_obj.Frontend.GRPCPoolMaxSize)
		}

		if config_obj.Frontend.GRPCPoolMaxWait > 0 {
			max_wait = int(config_obj.Frontend.GRPCPoolMaxWait)
		}
	}

	pool, err = grpcpool.NewWithContext(ctx,
		factory, 1, max_size, time.Duration(max_wait)*time.Second)
	if err != nil {
		return errors.Errorf(
			"Unable to connect to gRPC server: %v: %v", address, err)
	}
	return nil
}
