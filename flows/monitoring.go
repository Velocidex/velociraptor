// NOTE: This file implements the old Client communication protocol. It is
// here to provide backwards communication with older clients and will
// eventually be removed.

package flows

import (
	"bytes"
	"context"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	monitoringRowCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "received_monitoring_rows",
		Help: "Total number of event rows received from clients.",
	})
)

type jsonBatch struct {
	bytes.Buffer
	row_count int
}

// Receive monitoring messages from the client.
func MonitoringProcessMessage(
	ctx context.Context, config_obj *config_proto.Config,
	collection_context *CollectionContext,
	message *crypto_proto.VeloMessage) error {

	// Currently we do not do anything with monitoring status
	// messages so just ignore them.
	if message.Status != nil {
		return nil
	}

	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendUploadDataToFile(ctx,
			config_obj, collection_context, message)

	}

	response := message.VQLResponse
	if response == nil || response.Query == nil {
		return nil
	}

	// Deobfuscate the response if needed.
	_ = artifacts.Deobfuscate(config_obj, response)

	if response.Query.Name != "" {
		json_response := response.Response
		if json_response == "" {
			json_response = response.JSONLResponse
		}
		monitoringRowCounter.Add(float64(response.TotalRows))

		new_json_response := json.AppendJsonlItem(
			[]byte(json_response), "ClientId", message.Source)

		// Batch the rows to send together.
		collection_context.batchRows(response.Query.Name, new_json_response)
	}

	return nil
}

// Logs from monitoring flow need to be handled especially since they
// are written with a time index.
func flushContextLogsMonitoring(
	ctx context.Context, config_obj *config_proto.Config,
	collection_context *CollectionContext) error {

	// A single packet may have multiple log messages from
	// different artifacts. We cache the writers so we can send
	// the right message to the right log sink.
	writers := make(map[string]result_sets.TimedResultSetWriter)

	// Append logs to messages from previous packets.
	for _, row := range collection_context.Logs {
		artifact_name := row.Artifact
		if artifact_name == "" {
			artifact_name = "Unknown"
		}

		// Try to get the writer from the cache.
		rs_writer, pres := writers[artifact_name]
		if !pres {
			log_path_manager, err := artifact_paths.NewArtifactLogPathManager(ctx,
				config_obj, collection_context.ClientId,
				collection_context.SessionId, artifact_name)
			if err != nil {
				return err
			}

			// Write the logs asynchronously
			rs_writer, err = result_sets.NewTimedResultSetWriter(
				config_obj, log_path_manager, json.DefaultEncOpts(),
				utils.BackgroundWriter)
			if err != nil {
				return err
			}
			defer rs_writer.Close()

			writers[artifact_name] = rs_writer
		}

		rs_writer.Write(ordereddict.NewDict().
			Set("client_time", int64(row.Timestamp)/1000000).
			Set("level", row.Level).
			Set("message", row.Message))
	}

	// Clear the logs from the flow object.
	collection_context.Logs = nil
	return nil
}

func (self *CollectionContext) batchRows(
	artifact_name string, jsonl []byte) {
	batch, pres := self.monitoring_batch[artifact_name]
	if !pres {
		batch = &jsonBatch{}
	}
	batch.Write(jsonl)
	self.monitoring_batch[artifact_name] = batch
	self.Dirty = true
}

func flushMonitoringLogs(
	ctx context.Context, config_obj *config_proto.Config,
	collection_context *CollectionContext) error {

	journal, err := services.GetJournal(config_obj)
	if err != nil {
		return err
	}

	for query_name, jsonl_buff := range collection_context.monitoring_batch {
		err := journal.PushJsonlToArtifact(
			ctx,
			config_obj,
			jsonl_buff.Bytes(), jsonl_buff.row_count,
			query_name,
			collection_context.ClientId,
			collection_context.SessionId)
		if err != nil {
			return err
		}
	}
	return nil
}
