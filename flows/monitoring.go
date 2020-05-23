package flows

import (
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/result_sets"
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
	if response == nil {
		return nil
	}

	// Deobfuscate the response if needed.
	artifacts.Deobfuscate(config_obj, response)

	// Store the event log in the client's VFS.
	if response.Query.Name != "" {
		rows, err := utils.ParseJsonToDicts([]byte(response.Response))
		if err != nil {
			return err
		}

		// Mark the client this came from. Since message.Souce
		// is cryptographically trusted, this column may also
		// be trusted.
		for _, row := range rows {
			row.Set("ClientId", message.Source)
		}

		path_manager := result_sets.NewArtifactPathManager(config_obj,
			message.Source, message.SessionId, response.Query.Name)

		return services.GetJournal().PushRows(path_manager, rows)

	}

	return nil
}
