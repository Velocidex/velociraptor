package server

import (
	"errors"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	utils "www.velocidex.com/golang/velociraptor/testing"
)

func processWellKnownFlow(server *Server, message *crypto_proto.GrrMessage) bool {
	switch *message.SessionId {

	case "aff4:/flows/E:Enrol":
		err := enroll(server, message)
		if err != nil {
			server.Error("Enrol Error: %s", err)
		}
		return true

	default:
		return false
	}

}

func enroll(server *Server, message *crypto_proto.GrrMessage) error {
	csr, pres := responder.ExtractGrrMessagePayload(
		message).(*crypto_proto.Certificate)
	if !pres {
		return errors.New("Request should be of type Certificate")
	}

	utils.Debug(csr)

	if csr.GetType() == crypto_proto.Certificate_CSR && csr.Pem != nil {
		client_id, err := server.manager.AddCertificateRequest(csr.Pem)
		utils.Debug(client_id)
		if err != nil {
			return err
		}
	}

	return nil
}
