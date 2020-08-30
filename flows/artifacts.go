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
package flows

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

var (
	uploadCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "uploaded_files",
		Help: "Total number of Uploaded Files.",
	})

	uploadBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "uploaded_bytes",
		Help: "Total bytes of Uploaded Files.",
	})
)

// closeContext is called after all messages from the clients are
// processed in this group. Client messages are sent in groups inside
// the same POST request. Most of the time they belong to the same
// collection context. Therefore it makes sense to keep information in
// memory between processing individual messages. At the end of the
// processing we can close the context and flush data to disk.
func closeContext(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	// Context is not dirty - nothing to do.
	if !collection_context.Dirty || collection_context.ClientId == "" {
		return nil
	}

	// Decide if this collection exceeded its quota.
	err := checkContextResourceLimits(config_obj, collection_context)
	if err != nil {
		return err
	}

	if collection_context.StartTime == 0 {
		collection_context.StartTime = uint64(time.Now().UnixNano() / 1000)
	}

	collection_context.ActiveTime = uint64(time.Now().UnixNano() / 1000)

	if len(collection_context.Logs) > 0 {
		err := flushContextLogs(config_obj, collection_context)
		if err != nil {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = err.Error()
		}
	}

	if len(collection_context.UploadedFiles) > 0 {
		err := flushContextUploadedFiles(
			config_obj, collection_context)
		if err != nil {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = err.Error()
		}
	}
	collection_context.Dirty = false

	// Write the data before we fire the event.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = err.Error()
	}

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId, collection_context.SessionId)
	err = db.SetSubject(config_obj, flow_path_manager.Path(), collection_context)
	if err != nil {
		return err
	}

	// This is the final time we update the context - send a
	// journal message.
	if collection_context.Request != nil &&
		collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", collection_context).
			Set("FlowId", collection_context.SessionId).
			Set("ClientId", collection_context.ClientId)

		return services.GetJournal().PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{row},
			"System.Flow.Completion", collection_context.ClientId,
			collection_context.SessionId,
		)
	}

	return nil
}

func flushContextLogs(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId,
		collection_context.SessionId).Log()

	// Append logs to messages from previous packets.
	rs_writer, err := result_sets.NewResultSetWriter(
		config_obj, flow_path_manager, nil, false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range collection_context.Logs {
		rs_writer.Write(ordereddict.NewDict().
			Set("Timestamp", fmt.Sprintf("%v", row.Timestamp)).
			Set("time", time.Unix(int64(row.Timestamp)/1000000, 0).String()).
			Set("message", row.Message))
	}

	// Clear the logs from the flow object.
	collection_context.Logs = nil
	return nil
}

// Flush the logs to a csv file. This is important for long running
// flows with a lot of log messages.
func flushContextUploadedFiles(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId,
		collection_context.SessionId).UploadMetadata()

	rs_writer, err := result_sets.NewResultSetWriter(
		config_obj, flow_path_manager, nil, false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range collection_context.UploadedFiles {
		rs_writer.Write(ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("started", time.Now().UTC().String()).
			Set("vfs_path", row.Name).
			Set("file_size", row.Size).
			Set("uploaded_size", row.StoredSize))
	}

	// Clear the logs from the flow object.
	collection_context.UploadedFiles = nil
	return nil
}

// Load the collector context from storage.
func LoadCollectionContext(
	config_obj *config_proto.Config,
	client_id, flow_id string) (*flows_proto.ArtifactCollectorContext, error) {

	if flow_id == constants.MONITORING_WELL_KNOWN_FLOW {
		return &flows_proto.ArtifactCollectorContext{
			SessionId: flow_id,
			ClientId:  client_id,
		}, nil
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(config_obj, flow_path_manager.Path(),
		collection_context)
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, errors.New("Unknown flow " + client_id + " " + flow_id)
	}

	collection_context.Dirty = false

	return collection_context, nil
}

// Process an incoming message from the client.
func ArtifactCollectorProcessOneMessage(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) error {

	err := FailIfError(config_obj, collection_context, message)
	if err != nil {
		return err
	}

	// Check that this is not a retransmission - if it is we drop
	// it on the floor.
	if message.ResponseId < collection_context.NextResponseId {
		return nil
	}
	collection_context.NextResponseId = message.ResponseId + 1
	collection_context.Dirty = true

	// Handle the response depending on the RequestId
	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendUploadDataToFile(
			config_obj, collection_context, message)

	case constants.ProcessVQLResponses:
		completed, err := IsRequestComplete(
			config_obj, collection_context, message)
		if err != nil {
			return err
		}

		if completed {
			return nil
		}

		response := message.VQLResponse
		if response == nil || response.Query == nil {
			return errors.New("Expected args of type VQLResponse")
		}

		if collection_context == nil || collection_context.Request == nil {
			return errors.New("Invalid collection context")
		}

		err = artifacts.Deobfuscate(config_obj, response)
		if err != nil {
			return err
		}

		rows_written := uint64(0)
		if response.Query.Name != "" {
			path_manager := result_sets.NewArtifactPathManager(config_obj,
				collection_context.Request.ClientId,
				collection_context.SessionId,
				response.Query.Name)

			rs_writer, err := result_sets.NewResultSetWriter(
				config_obj, path_manager, nil, false /* truncate */)

			// Support the old clients which send JSON
			// array responses. We need to decode the JSON
			// response, then re-encode it into JSONL for
			// log files.
			if len(response.Response) > 0 {
				if err != nil {
					return err
				}
				defer rs_writer.Close()

				rows, err := utils.ParseJsonToDicts([]byte(
					response.Response))
				if err != nil {
					return err
				}

				for _, row := range rows {
					rows_written++
					rs_writer.Write(row)
				}

				// New clients already encode the JSON
				// as line delimited, so we only need
				// to append to end of the log file -
				// much faster!
			} else if len(response.JSONLResponse) > 0 {
				rs_writer.WriteJSONL([]byte(response.JSONLResponse))
				rows_written = response.TotalRows
			}

			// Update the artifacts with results in the
			// context.
			if rows_written > 0 {
				if !utils.InString(collection_context.ArtifactsWithResults,
					response.Query.Name) {
					collection_context.ArtifactsWithResults = append(
						collection_context.ArtifactsWithResults,
						response.Query.Name)
				}
				collection_context.TotalCollectedRows += rows_written
				collection_context.Dirty = true
			}
		}
	}

	return nil
}

func IsRequestComplete(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) (bool, error) {

	// Nope request is not complete.
	if message.Status == nil {
		return false, nil
	}

	// Complete the collection
	if collection_context == nil || collection_context.Request == nil {
		return false, errors.New("Invalid collection context")
	}

	// Only terminate a running flow.
	if collection_context.State == flows_proto.ArtifactCollectorContext_RUNNING {

		// Update any hunts if needed.
		if constants.HuntIdRegex.MatchString(collection_context.Request.Creator) {
			err := services.GetHuntDispatcher().ModifyHunt(
				collection_context.Request.Creator,
				func(hunt *api_proto.Hunt) error {
					if hunt != nil && hunt.Stats != nil {
						hunt.Stats.TotalClientsWithResults++
					}
					return nil
				})
			if err != nil {
				return true, err
			}
		}

		collection_context.ExecutionDuration = message.Status.Duration
		collection_context.State = flows_proto.ArtifactCollectorContext_FINISHED
		collection_context.Dirty = true
	}

	return true, nil
}

func FailIfError(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) error {

	// Not a status message
	if message.Status == nil {
		return nil
	}

	// If the status is OK then we do not fail the flow.
	if message.Status.Status == crypto_proto.GrrStatus_OK {
		return nil
	}

	if collection_context == nil || collection_context.Request == nil {
		return errors.New("Invalid collection context")
	}

	// Only terminate a running flows.
	if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return errors.New(message.Status.ErrorMessage)
	}

	collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
	collection_context.ActiveTime = uint64(time.Now().UnixNano() / 1000)
	collection_context.Status = message.Status.ErrorMessage
	collection_context.Backtrace = message.Status.Backtrace
	collection_context.ExecutionDuration = message.Status.Duration
	collection_context.Dirty = true

	// Update the hunt stats if this is a hunt.
	if constants.HuntIdRegex.MatchString(collection_context.Request.Creator) {
		err := services.GetHuntDispatcher().ModifyHunt(
			collection_context.Request.Creator,
			func(hunt *api_proto.Hunt) error {
				if hunt != nil && hunt.Stats != nil {
					hunt.Stats.TotalClientsWithErrors++
				}
				return nil
			})
		if err != nil {
			return err
		}
	}

	return errors.New(message.Status.ErrorMessage)
}

func appendUploadDataToFile(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) error {

	file_buffer := message.FileBuffer
	if file_buffer == nil || file_buffer.Pathspec == nil {
		return errors.New("Expected args of type FileBuffer")
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	flow_path_manager := paths.NewFlowPathManager(
		message.Source, collection_context.SessionId)

	// Figure out where to store the file.
	file_path_manager := flow_path_manager.GetUploadsFile(
		file_buffer.Pathspec.Accessor,
		file_buffer.Pathspec.Path)

	fd, err := file_store_factory.WriteFile(file_path_manager.Path())
	if err != nil {
		// If we fail to write this one file we keep going -
		// otherwise the flow will be terminated.
		Log(config_obj, collection_context,
			fmt.Sprintf("While writing to %v: %v",
				file_path_manager.Path(), err))
		return nil
	}
	defer fd.Close()

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {
		err = fd.Truncate()
		if err != nil {
			return err
		}

		// If the file is sparse we can store a different
		// amount from the actual file size. Therefore in that
		// case we expect less bytes to be sent.
		size := file_buffer.Size
		if file_buffer.IsSparse {
			size = file_buffer.StoredSize
		}

		collection_context.TotalUploadedFiles += 1
		collection_context.TotalExpectedUploadedBytes += size
		collection_context.UploadedFiles = append(
			collection_context.UploadedFiles,
			&flows_proto.ArtifactUploadedFileInfo{
				Name:       file_path_manager.Path(),
				Size:       file_buffer.Size,
				StoredSize: size,
			})
		collection_context.Dirty = true
	}

	if len(file_buffer.Data) > 0 {
		collection_context.TotalUploadedBytes += uint64(len(file_buffer.Data))
		collection_context.Dirty = true
	}

	_, err = fd.Write(file_buffer.Data)
	if err != nil {
		Log(config_obj, collection_context,
			fmt.Sprintf("While writing to %v: %v",
				file_path_manager.Path(), err))
		return nil
	}

	// Does this packet have an index? It could be sparse.
	if file_buffer.Index != nil {
		fd, err := file_store_factory.WriteFile(file_path_manager.IndexPath())
		if err != nil {
			return err
		}
		defer fd.Close()

		err = fd.Truncate()
		if err != nil {
			return err
		}

		data := json.MustMarshalIndent(file_buffer.Index)
		_, err = fd.Write(data)
		if err != nil {
			return err
		}

		collection_context.UploadedFiles = append(
			collection_context.UploadedFiles,
			&flows_proto.ArtifactUploadedFileInfo{
				Name:       file_path_manager.IndexPath(),
				Size:       uint64(len(data)),
				StoredSize: uint64(len(data)),
			})
		collection_context.Dirty = true
	}

	// When the upload completes, we emit an event.
	if file_buffer.Eof {
		uploadCounter.Inc()
		uploadBytes.Add(float64(file_buffer.StoredSize))

		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("ClientId", message.Source).
			Set("VFSPath", file_path_manager.Path()).
			Set("UploadName", file_buffer.Pathspec.Path).
			Set("Accessor", file_buffer.Pathspec.Accessor).
			Set("Size", file_buffer.Size).
			Set("UploadedSize", file_buffer.StoredSize)

		return services.GetJournal().PushRowsToArtifact(config_obj,
			[]*ordereddict.Dict{row},
			"System.Upload.Completion", message.Source, collection_context.SessionId,
		)
	}

	return nil
}

func Log(config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	log_msg string) {
	log_msg = artifacts.DeobfuscateString(config_obj, log_msg)
	collection_context.Logs = append(
		collection_context.Logs, &crypto_proto.LogMessage{
			Message:   log_msg,
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		})
	collection_context.Dirty = true
}

type FlowRunner struct {
	context_map map[string]*flows_proto.ArtifactCollectorContext
	config_obj  *config_proto.Config
}

func NewFlowRunner(config_obj *config_proto.Config) *FlowRunner {
	return &FlowRunner{
		config_obj:  config_obj,
		context_map: make(map[string]*flows_proto.ArtifactCollectorContext),
	}
}

func (self *FlowRunner) Close() {
	for _, collection_context := range self.context_map {
		err := closeContext(self.config_obj, collection_context)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("While closing flow %v for client %v: %v",
				collection_context.SessionId, collection_context.ClientId, err)
		}
	}
}

func (self *FlowRunner) ProcessSingleMessage(
	ctx context.Context,
	job *crypto_proto.GrrMessage) {

	// Foreman messages are related to hunts.
	if job.ForemanCheckin != nil {
		err := ForemanProcessMessage(
			ctx, self.config_obj,
			job.Source, job.ForemanCheckin)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("ForemanCheckin for client %v: %v", job.Source, err)
		}
		return
	}

	// CSR messages are related to enrolment. By the time the
	// message arrives here, it is authenticated and the client is
	// fully enrolled so it serves no purpose here - Just ignore it.
	if job.CSR != nil {
		return
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	collection_context, pres := self.context_map[job.SessionId]
	if !pres {
		var err error

		// Only process real flows.
		if !strings.HasPrefix(job.SessionId, "F.") {
			logger.Error("Invalid job SessionId %v", job.SessionId)
			return
		}

		collection_context, err = LoadCollectionContext(
			self.config_obj, job.Source, job.SessionId)
		if err != nil {
			// Ignore logs and status messages from the
			// client. These are generated by cancel
			// requests anyway.
			if job.LogMessage != nil || job.Status != nil {
				return
			}

			logger.Error(fmt.Sprintf("Unable to load flow %s: %v", job.SessionId, err))

			db, err := datastore.GetDB(self.config_obj)
			if err == nil {
				err := db.QueueMessageForClient(self.config_obj, job.Source,
					&crypto_proto.GrrMessage{
						Cancel:    &crypto_proto.Cancel{},
						SessionId: job.SessionId,
					})
				if err != nil {
					logger.Error("Queueing for client %v: %v",
						job.Source, err)
				}
			}
			return
		}
		self.context_map[job.SessionId] = collection_context
	}

	if collection_context == nil {
		return
	}

	if job.LogMessage != nil {
		Log(self.config_obj, collection_context, job.LogMessage.Message)
		return
	}

	if job.SessionId == constants.MONITORING_WELL_KNOWN_FLOW {
		err := MonitoringProcessMessage(self.config_obj, collection_context, job)
		if err != nil {
			Log(self.config_obj, collection_context,
				fmt.Sprintf("MonitoringProcessMessage: %v", err))
		}
		return
	}

	err := ArtifactCollectorProcessOneMessage(
		self.config_obj, collection_context, job)
	if err != nil {
		Log(self.config_obj, collection_context,
			fmt.Sprintf("While processing job %v", err))
	}
}

func (self *FlowRunner) ProcessMessages(ctx context.Context,
	message_info *crypto.MessageInfo) (err error) {

	return message_info.IterateJobs(ctx, self.ProcessSingleMessage)
}
