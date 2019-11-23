package flows

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
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

	// Write the response on the journal.
	gJournalWriter.Channel <- &Event{
		Config:    config_obj,
		Timestamp: time.Now(),
		ClientId:  message.Source,
		QueryName: response.Query.Name,
		Response:  response.Response,
		Columns:   response.Columns,
	}

	// Store the event log in the client's VFS.
	if response.Query.Name != "" {
		file_store_factory := file_store.GetFileStore(config_obj)

		artifact_name, source_name := artifacts.
			QueryNameToArtifactAndSource(
				response.Query.Name)

		log_path := artifacts.GetCSVPath(
			message.Source, /* client_id */
			artifacts.GetDayName(),
			collection_context.SessionId,
			artifact_name, source_name,
			artifacts.MODE_MONITORING_DAILY)

		fd, err := file_store_factory.WriteFile(log_path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
		defer fd.Close()

		writer, err := csv.GetCSVWriter(vql_subsystem.MakeScope(), fd)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
		defer writer.Close()

		var rows []map[string]interface{}
		err = json.Unmarshal([]byte(response.Response), &rows)
		if err != nil {
			return errors.WithStack(err)
		}

		for _, row := range rows {
			csv_row := ordereddict.NewDict().Set(
				"_ts", int(time.Now().Unix()))
			for _, column := range response.Columns {
				csv_row.Set(column, row[column])
			}

			writer.Write(csv_row)
		}
	}

	return nil
}
