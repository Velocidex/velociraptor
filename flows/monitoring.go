package flows

import (
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
)

func MonitoringProcessMessage(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) error {

	err := FailIfError(config_obj, collection_context, message)
	if err != nil {
		return err
	}

	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendUploadDataToFile(
			config_obj, collection_context, message)

	}

	response := message.VQLResponse
	if response == nil {
		return nil
	}

	// Deobfuscate the response if needed.
	err = artifacts.Deobfuscate(config_obj, response)
	if err != nil {
		return err
	}

	// Store the event log in the client's VFS.
	if response.Query.Name != "" {
		return services.GetJournal().Push(
			response.Query.Name, message.Source,
			paths.MODE_MONITORING_DAILY, []byte(response.Response))

	}

	return nil
}
