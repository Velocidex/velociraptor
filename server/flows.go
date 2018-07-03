package server

import (
	"errors"
	"github.com/golang/protobuf/ptypes"
	"strings"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
)

func enroll(server *Server, message *crypto_proto.GrrMessage) error {
	csr, pres := responder.ExtractGrrMessagePayload(
		message).(*crypto_proto.Certificate)
	if !pres {
		return errors.New("Request should be of type Certificate")
	}

	if csr.GetType() == crypto_proto.Certificate_CSR && csr.Pem != nil {
		client_urn, err := server.manager.AddCertificateRequest(csr.Pem)
		if err != nil {
			return err
		}

		client_id := strings.TrimPrefix(*client_urn, "aff4:/")
		flow_runner_args := &flows_proto.FlowRunnerArgs{
			ClientId: client_id,
			FlowName: "VInterrogate",
		}

		args := &flows_proto.VInterrogateArgs{}
		marshalled_args, err := ptypes.MarshalAny(args)
		if err != nil {
			return err
		}
		flow_runner_args.Args = marshalled_args

		_, err = flows.StartFlow(server.config, flow_runner_args)
		if err != nil {
			return err
		}
	}

	return nil
}
