package server

import (
	"context"
	"errors"
	"strings"

	"github.com/golang/protobuf/ptypes"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
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
			FlowName: "VInterrogate",
		}
		flow_args, err := ptypes.MarshalAny(&flows_proto.VInterrogateArgs{})
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
