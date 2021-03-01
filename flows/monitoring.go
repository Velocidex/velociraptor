package flows

import (
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

// Receive monitoring messages from the client.
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
	if response == nil || response.Query == nil {
		return nil
	}

	// Deobfuscate the response if needed.
	_ = artifacts.Deobfuscate(config_obj, response)

	// Store the event log in the client's VFS.
	if response.Query.Name != "" {
		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}

		// We need to parse each event since it needs to be
		// pushed to the journal, in case a reader is
		// listening to it. FIXME: This is expensive CPU wise,
		// we need to think of a better way to do this.
		rows, err := utils.ParseJsonToDicts([]byte(json_response))
		if err != nil {
			return err
		}

		// Mark the client this came from. Since message.Souce
		// is cryptographically trusted, this column may also
		// be trusted.
		for _, row := range rows {
			row.Set("ClientId", message.Source)
		}
		journal, err := services.GetJournal()
		if err != nil {
			return err
		}

		return journal.PushRowsToArtifact(
			config_obj, rows, response.Query.Name,
			message.Source, message.SessionId)

	}

	return nil
}
