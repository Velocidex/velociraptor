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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
)

var (
	// Cache the creds for internal gRPC connections.
	mu    sync.Mutex
	creds credentials.TransportCredentials
)

func getCreds(config_obj *api_proto.Config) credentials.TransportCredentials {
	mu.Lock()
	defer mu.Unlock()

	if creds == nil {
		// We use the Frontend's certificate because this connection
		// represents an internal connection.
		cert, err := tls.X509KeyPair(
			[]byte(config_obj.Frontend.Certificate),
			[]byte(config_obj.Frontend.PrivateKey))
		if err != nil {
			return nil
		}

		// The server cert must be signed by our CA.
		CA_Pool := x509.NewCertPool()
		CA_Pool.AppendCertsFromPEM([]byte(config_obj.Client.CaCertificate))

		creds = credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      CA_Pool,
			ServerName:   constants.FRONTEND_NAME,
		})
	}

	return creds
}

// TODO- Return a cluster dialer.
func GetChannel(config_obj *api_proto.Config) *grpc.ClientConn {
	address := GetAPIConnectionString(config_obj)
	con, err := grpc.Dial(address, grpc.WithTransportCredentials(
		getCreds(config_obj)))
	if err != nil {
		panic(fmt.Sprintf("Unable to connect to self: %v: %v", address, err))
	}
	return con
}

func GetAPIConnectionString(config_obj *api_proto.Config) string {
	switch config_obj.API.BindScheme {
	case "tcp":
		return fmt.Sprintf("%s:%d", config_obj.API.BindAddress,
			config_obj.API.BindPort)

	case "unix":
		return fmt.Sprintf("unix://%s", config_obj.API.BindAddress)
	}

	panic("Unknown API.BindScheme")
}
