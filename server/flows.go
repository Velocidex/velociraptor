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
package server

import (
	"context"
	"errors"
	"strings"

	"github.com/golang/protobuf/ptypes"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/responder"
)

func enroll(server *Server, message *crypto_proto.GrrMessage) error {
	csr, pres := responder.ExtractGrrMessagePayload(
		message).(*crypto_proto.Certificate)
	if !pres {
		return errors.New("request should be of type Certificate")
	}

	if csr.GetType() == crypto_proto.Certificate_CSR && csr.Pem != nil {
		client_urn, err := server.manager.AddCertificateRequest(csr.Pem)
		if err != nil {
			return err
		}

		client_id := strings.TrimPrefix(*client_urn, "aff4:/")

		channel := grpc_client.GetChannel(server.config)
		defer channel.Close()

		client := api_proto.NewAPIClient(channel)

		flow_runner_args := &flows_proto.FlowRunnerArgs{
			ClientId: client_id,
			FlowName: "ArtifactCollector",
		}

		flow_args, err := ptypes.MarshalAny(&flows_proto.ArtifactCollectorArgs{
			Artifacts: &flows_proto.Artifacts{
				Names: []string{constants.CLIENT_INFO_ARTIFACT},
			},
		})
		if err != nil {
			return err
		}
		flow_runner_args.Args = flow_args

		_, err = client.LaunchFlow(context.Background(), flow_runner_args)
		if err != nil {
			return err
		}
	}

	return nil
}
