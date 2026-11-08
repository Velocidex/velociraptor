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
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

// The different types of caller supported. This affects the user
// identity we use when making the API call.
type CallerIdentity string

const (
	// Used by the gateway proxy to call the API on behalf of the
	// GUI. This identity causes the real user identity to be
	// recovered from the embedded metadata.
	GRPC_GW CallerIdentity = "GRPC_GW"

	// Used by user API calls
	API_User CallerIdentity = "API_User"

	// Used by the Velociraptor Server to make minion to master API
	// calls. Implicitly trusted.
	SuperUser CallerIdentity = "SuperUser"

	// Org identities are prefixed by "Org-"
)

var (
	Factory APIClientFactory = NewDirectConnectionClient()

	grpcCallCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "grpc_client_calls",
		Help: "Total number of internal gRPC calls.",
	})

	grpcStubs = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "grpc_stubs",
		Help: "Total current number of stubs.",
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

type APIClientFactory interface {
	GetAPIClientForOrgRedirect(
		ctx context.Context,
		config_obj *config_proto.Config) (
		api_proto.APIClient, func() error, error)

	GetAPIClient(
		ctx context.Context,
		identity CallerIdentity,
		config_obj *config_proto.Config) (
		api_proto.APIClient, func() error, error)
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
		return fmt.Sprintf("%s:%d", GetAPIHostname(config_obj),
			config_obj.API.BindPort)

	case "unix":
		return fmt.Sprintf("unix://%s", config_obj.API.BindAddress)
	}

	panic("Unknown API.BindScheme")
}

func GetAPIHostname(config_obj *config_proto.Config) string {
	hostname := config_obj.API.Hostname

	// Prefer connecting over the loopback if possible.
	if config_obj.API.BindAddress == "127.0.0.1" {
		hostname = config_obj.API.BindAddress
	}

	// If there is not specifically a hostname here, we try the
	// frontend's hostname
	if hostname == "" {
		hostname = config_obj.Frontend.Hostname
	}

	// Failing that we try to bind to the API BindAddress - this
	// only works if the interface is routable from here (i.e. it
	// will fail on minions which are from another server).
	if hostname == "" {
		hostname = config_obj.API.BindAddress
	}

	return hostname
}
