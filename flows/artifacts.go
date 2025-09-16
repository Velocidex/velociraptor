// NOTE: This file implements the old Client communication protocol. It is
// here to provide backwards communication with older clients and will
// eventually be removed.

/*
Velociraptor - Dig Deeper
Copyright (C) 2019-2025 Rapid7 Inc.

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
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/proto"
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
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
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

	invalidSessionId = errors.New("Invalid SessionId")
	invalidClientId  = errors.New("Invalid ClientId")
)

// The CollectionContext tracks collections as they are being
// processed. The client send back a bunch of results consisting of
// logs, monitoring results, status errors etc. As the server
// processes these, it loads the CollectionContext from the datastore,
// and updates the CollectionContext state. When the server completes
// processing, the CollectionContext context is flushed by to the
// filestore.
type CollectionContext struct {
	mu sync.Mutex

	flows_proto.ArtifactCollectorContext

	// We batch all monitoring JSONL rows and then flush them at the
	// end.
	monitoring_batch map[string]*jsonBatch

	// The completer keeps track of all asynchronous filesystem
	// operations that will occur so that when everything is written
	// to disk, the completer can send the System.Flow.Completion
	// event. This is important as we dont want watchers of
	// System.Flow.Completion to attempt to open the collection before
	// everything is written.
	completer *utils.Completer

	// Indicate if the System.Flow.Completion should be sent. This
	// only happens once the collection is complete and results are
	// written. It only happens at most once per collection.
	send_update bool
}

func NewCollectionContext(
	ctx context.Context, config_obj *config_proto.Config) *CollectionContext {
	self := &CollectionContext{
		ArtifactCollectorContext: flows_proto.ArtifactCollectorContext{},
		monitoring_batch:         make(map[string]*jsonBatch),
	}

	// If we need to send a notification we should wait until all parts of
	// the collection are fully stored first to avoid a race with any
	// listeners on System.Flow.Completion.
	self.completer = utils.NewCompleter(func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		// Mark the collection as updated.
		_ = updateContext(config_obj, self.ClientId, self.SessionId)

		if !self.send_update {
			return
		}
		// Do not send it again.
		self.send_update = false

		// If this is the final response (i.e. the flow is not running)
		// and we have not yet sent an update, then we will notify a flow
		// completion.
		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", proto.Clone(&self.ArtifactCollectorContext)).
			Set("FlowId", self.SessionId).
			Set("ClientId", self.ClientId)

		journal, err := services.GetJournal(config_obj)
		if err == nil {
			journal.PushRowsToArtifactAsync(ctx,
				config_obj, row, "System.Flow.Completion")
		}
	})

	return self
}

// Flush the context object to disk. This must happen AFTER all data
// is written
func updateContext(
	config_obj *config_proto.Config,
	client_id, flow_id string) error {

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return err
	}

	ping_record := &flows_proto.PingContext{
		ActiveTime: uint64(time.Now().UnixNano() / 1000),
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	// Just a blind write.
	return db.SetSubjectWithCompletion(
		config_obj, flow_path_manager.Ping(),
		ping_record, nil)
}

// closeContext is called after all messages from the clients are
// processed in this group. Client messages are sent in groups inside
// the same POST request. Most of the time they belong to the same
// collection context. Therefore it makes sense to keep information in
// memory between processing individual messages. At the end of the
// processing we can close the context and flush data to disk.

// NOTE: It is possible that messages arrive AFTER the collection is
// completed. Clients send a STATUS message when the entire result set
// is complete, but query logs may arrive after the query is
// complete. Usually they arrive in the same packet (and so will be
// processed before the closeContext() but on very loaded systems it
// is possible the log messages arrive in a separate packet. We
// therefore ensure that we only send a System.Flow.Completion message
// once a status is received and not again.
func closeContext(
	ctx context.Context,
	config_obj *config_proto.Config,
	collection_context *CollectionContext) error {

	// Ensure the completion is not fired until we are done here
	// completely.
	completion_func := collection_context.completer.GetCompletionFunc()
	defer completion_func()

	// Context is not dirty - nothing to do.
	if !collection_context.Dirty || collection_context.ClientId == "" {
		return nil
	}

	// Decide if this collection exceeded its quota.
	err := checkContextResourceLimits(ctx, config_obj, collection_context)
	if err != nil {
		return err
	}

	// Update the start time if needed.
	if collection_context.StartTime == 0 {
		collection_context.StartTime = uint64(time.Now().UnixNano() / 1000)
	}

	// Mark the flow as last active now.
	collection_context.ActiveTime = uint64(time.Now().UnixNano() / 1000)

	// Figure out if we will send a System.Flow.Completion after
	// this. This depends on:
	// 1. We do not expect any more responses
	// 2. We have not sent a System.Flow.Completion yet.
	//
	// It is important to delay the System.Flow.Completion message
	// until after all artifacts have been collected so users can
	// watch them and expect to see results.
	if collection_context.Request != nil &&
		int64(len(collection_context.QueryStats)) >= collection_context.TotalRequests &&
		!collection_context.UserNotified {

		// Record the message was sent - so we never resend the
		// message, even with new data.
		collection_context.UserNotified = true

		// Instruct the completion function to send the message.
		collection_context.send_update = true
		collection_context.Dirty = true
	}

	if len(collection_context.Logs) > 0 {
		err := flushContextLogs(ctx,
			config_obj, collection_context, collection_context.completer)
		if err != nil {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = err.Error()
			collection_context.Dirty = true
		}
	}

	if len(collection_context.UploadedFiles) > 0 {
		err := flushContextUploadedFiles(
			config_obj, collection_context, collection_context.completer)
		if err != nil {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = err.Error()
			collection_context.Dirty = true
		}
	}

	if len(collection_context.monitoring_batch) > 0 {
		err = flushMonitoringLogs(ctx, config_obj, collection_context)
		if err != nil {
			collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
			collection_context.Status = err.Error()
			collection_context.Dirty = true
		}
	}

	collection_context.Dirty = false

	// Write the data before we fire the event so the data is
	// available to any listeners of the event.
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = err.Error()
	}

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId, collection_context.SessionId)

	return db.SetSubjectWithCompletion(
		config_obj, flow_path_manager.Path(),
		collection_context, collection_context.completer.GetCompletionFunc())
}

func flushContextUploadedFiles(
	config_obj *config_proto.Config,
	collection_context *CollectionContext,
	completion *utils.Completer) error {

	flow_path_manager := paths.NewFlowPathManager(
		collection_context.ClientId,
		collection_context.SessionId).UploadMetadata()

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, flow_path_manager,
		nil, /* opts */
		completion.GetCompletionFunc(),
		false /* truncate */)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	for _, row := range collection_context.UploadedFiles {
		rs_writer.Write(ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("started", time.Now().UTC().String()).
			Set("vfs_path", row.Name).
			Set("_Components", row.Components).
			Set("file_size", row.Size).
			Set("uploaded_size", row.StoredSize))
	}

	// Clear the logs from the flow object.
	collection_context.UploadedFiles = nil
	return nil
}

// Load the collector context from storage.
func LoadCollectionContext(
	ctx context.Context, config_obj *config_proto.Config,
	client_id, flow_id string) (*CollectionContext, error) {

	if flow_id == constants.MONITORING_WELL_KNOWN_FLOW {
		result := NewCollectionContext(ctx, config_obj)
		result.SessionId = flow_id
		result.ClientId = client_id

		return result, nil
	}

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	collection_context := NewCollectionContext(ctx, config_obj)
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(config_obj, flow_path_manager.Path(),
		&collection_context.ArtifactCollectorContext)
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, errors.New("Unknown flow " + client_id + " " + flow_id)
	}
	collection_context.TotalLoads++
	collection_context.Dirty = false

	return collection_context, nil
}

// Process an incoming message from the client.
func ArtifactCollectorProcessOneMessage(
	ctx context.Context, config_obj *config_proto.Config,
	collection_context *CollectionContext,
	message *crypto_proto.VeloMessage) error {

	if message.Status != nil {
		return CheckForStatus(config_obj, collection_context, message)
	}

	// Ignore new style messages
	if message.UploadTransaction != nil {
		return nil
	}

	// Check that this is not a retransmission - if it is we drop
	// it on the floor. Backwards compatibility - older clients
	// increment response id from 0 but newer clients use nano
	// timestamp.
	if message.ResponseId > 100000 &&
		message.ResponseId < collection_context.NextResponseId {
		return nil
	}
	collection_context.NextResponseId = message.ResponseId + 1
	collection_context.Dirty = true

	// Handle the response depending on the RequestId
	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendUploadDataToFile(ctx,
			config_obj, collection_context, message)

	case constants.ProcessVQLResponses:
		response := message.VQLResponse
		if response == nil || response.Query == nil {
			return fmt.Errorf("Expected args of type VQLResponse not %v", message)
		}

		if collection_context == nil || collection_context.Request == nil {
			return errors.New("Invalid collection context")
		}

		err := artifacts.Deobfuscate(config_obj, response)
		if err != nil {
			return err
		}

		rows_written := uint64(0)
		if response.Query.Name != "" {
			path_manager, err := artifact_paths.NewArtifactPathManager(
				ctx, config_obj,
				collection_context.Request.ClientId,
				collection_context.SessionId,
				response.Query.Name)
			if err != nil {
				return err
			}

			file_store_factory := file_store.GetFileStore(config_obj)
			rs_writer, err := result_sets.NewResultSetWriter(
				file_store_factory,
				path_manager.Path(),
				nil, /* opts */
				collection_context.completer.GetCompletionFunc(),
				false /* truncate */)
			if err != nil {
				return err
			}
			defer rs_writer.Close()

			// Support the old clients which send JSON
			// array responses. We need to decode the JSON
			// response, then re-encode it into JSONL for
			// log files.
			if len(response.Response) > 0 {
				rows, err := utils.ParseJsonToDicts([]byte(
					response.Response))
				if err != nil {
					return err
				}

				for _, row := range rows {
					rows_written++
					rs_writer.Write(row)
					rowCounter.Inc()
				}

				// New clients already encode the JSON
				// as line delimited, so we only need
				// to append to end of the log file -
				// much faster!
			} else if len(response.JSONLResponse) > 0 {
				rs_writer.WriteJSONL(
					[]byte(response.JSONLResponse), response.TotalRows)
				rows_written = response.TotalRows
				rowCounter.Add(float64(response.TotalRows))
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

func CheckForStatus(
	config_obj *config_proto.Config,
	collection_context *CollectionContext,
	message *crypto_proto.VeloMessage) error {

	// Not a status message
	if message.Status == nil {
		return nil
	}

	if collection_context == nil || collection_context.Request == nil {
		return errors.New("Invalid collection context")
	}

	// Update our record of all the status messages from this
	// collection.
	updateQueryStats(config_obj, collection_context, message.Status)

	// Update the active time for each response.
	collection_context.ActiveTime = uint64(time.Now().UnixNano() / 1000)

	collection_context.Dirty = true

	// If the status is OK then we do not fail the flow.
	if message.Status.Status == crypto_proto.VeloStatus_OK {
		return nil
	}
	return errors.New(message.Status.ErrorMessage)
}

func appendUploadDataToFile(
	ctx context.Context, config_obj *config_proto.Config,
	collection_context *CollectionContext,
	message *crypto_proto.VeloMessage) error {

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
		file_buffer.Pathspec.Path,
		file_buffer.Pathspec.Components)

	fd, err := file_store_factory.WriteFile(file_path_manager.Path())
	if err != nil {
		// If we fail to write this one file we keep going -
		// otherwise the flow will be terminated.
		Log(config_obj, collection_context,
			fmt.Sprintf("While writing to %v: %v",
				file_path_manager.Path().AsClientPath(), err))
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
				Name:       file_path_manager.VisibleVFSPath(),
				Components: file_path_manager.Path().Components(),
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
				file_path_manager.Path().AsClientPath(), err))
		return nil
	}

	// Does this packet have an index? It could be sparse.
	if file_buffer.Index != nil {
		fd, err := file_store_factory.WriteFile(
			file_path_manager.IndexPath())
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
				Name:       file_path_manager.VisibleVFSPath() + ".idx",
				Components: file_path_manager.IndexPath().Components(),
				Size:       uint64(len(data)),
				StoredSize: uint64(len(data)),
				Type:       "idx",
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
			Set("VFSPath", file_path_manager.Path().AsClientPath()).
			Set("UploadName", file_buffer.Pathspec.Path).
			Set("Accessor", file_buffer.Pathspec.Accessor).
			Set("Size", file_buffer.Size).
			Set("UploadedSize", file_buffer.StoredSize)

		journal, err := services.GetJournal(config_obj)
		if err != nil {
			return err
		}

		return journal.PushRowsToArtifact(ctx, config_obj,
			[]*ordereddict.Dict{row},
			"System.Upload.Completion",
			message.Source, collection_context.SessionId,
		)
	}

	return nil
}

// Generate a flow log from a client LogMessage proto. Deobfuscates
// the message.
func LogMessage(config_obj *config_proto.Config,
	collection_context *CollectionContext,
	msg *crypto_proto.LogMessage) error {
	log_msg := artifacts.DeobfuscateString(config_obj, msg.Message)
	artifact_name := artifacts.DeobfuscateString(config_obj, msg.Artifact)

	// Newer clients send a combined log JSONL packet which we can
	// just write at once.
	if len(msg.Jsonl) > 0 {
		return writeLogMessages(config_obj, collection_context, msg)
	}

	collection_context.Logs = append(
		collection_context.Logs, &crypto_proto.LogMessage{
			Message:   log_msg,
			Level:     msg.Level,
			Artifact:  artifact_name,
			Timestamp: msg.Timestamp,
		})
	collection_context.Dirty = true

	return nil
}

func Log(config_obj *config_proto.Config,
	collection_context *CollectionContext,
	log_msg string) {
	log_msg = artifacts.DeobfuscateString(config_obj, log_msg)
	collection_context.Logs = append(
		collection_context.Logs, &crypto_proto.LogMessage{
			Message:   log_msg,
			Level:     logging.DEFAULT,
			Timestamp: uint64(time.Now().UTC().UnixNano() / 1000),
		})
	collection_context.Dirty = true
}

type FlowRunner struct {
	mu sync.Mutex

	context_map map[string]*CollectionContext
	config_obj  *config_proto.Config
}

// A flow runner compatible with old clients. This runner manages flow
// state on the server which is less efficient. Modern clients
// (>0.6.7) will manage flows client side and so we will use
// the more efficient ClientFlowRunner instead.
func NewLegacyFlowRunner(config_obj *config_proto.Config) *FlowRunner {
	return &FlowRunner{
		config_obj:  config_obj,
		context_map: make(map[string]*CollectionContext),
	}
}

func (self *FlowRunner) Close(ctx context.Context) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, collection_context := range self.context_map {
		err := closeContext(ctx, self.config_obj, collection_context)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("While closing flow %v for client %v: %v",
				collection_context.SessionId, collection_context.ClientId, err)
		}
	}
}

func (self *FlowRunner) ProcessSingleMessage(
	ctx context.Context,
	job *crypto_proto.VeloMessage) error {

	// Only process real flows.
	if !strings.HasPrefix(job.SessionId, "F.") {
		return nil
	}

	// json.TraceMessage(job.Source+"_job", job)

	// CSR messages are related to enrolment. By the time the
	// message arrives here, it is authenticated and the client is
	// fully enrolled so it serves no purpose here - Just ignore it.
	if job.CSR != nil {
		return nil
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	collection_context, pres := self.context_map[job.SessionId]
	if !pres {
		var err error

		if job.SessionId == "" {
			logger.Error("Empty SessionId: %v", job)
			return invalidSessionId
		}

		// Should never happen because source is filled in from the
		// crypto envelope.
		if job.Source == "" {
			logger.Error("Empty Source: %v", job)
			return invalidClientId
		}

		collection_context, err = LoadCollectionContext(ctx,
			self.config_obj, job.Source, job.SessionId)
		if err != nil {
			// Ignore logs and status messages from the
			// client. These are generated by cancel
			// requests anyway.
			if job.LogMessage != nil || job.Status != nil {
				return nil
			}

			logger.Error("Unable to load flow %s: %v", job.SessionId, err)

			client_manager, err := services.GetClientInfoManager(self.config_obj)
			if err != nil {
				return err
			}

			// Flow does not exist cancel the corresponding flow on the client.
			err = client_manager.QueueMessageForClient(ctx, job.Source,
				&crypto_proto.VeloMessage{
					Cancel:    &crypto_proto.Cancel{},
					SessionId: job.SessionId,
				},
				services.NOTIFY_CLIENT, utils.BackgroundWriter)
			if err != nil {
				return fmt.Errorf("Queueing for client %v: %w",
					job.Source, err)
			}
			return nil
		}
		self.context_map[job.SessionId] = collection_context
	}

	if collection_context == nil {
		return nil
	}

	if job.LogMessage != nil {
		err := LogMessage(self.config_obj, collection_context, job.LogMessage)
		if err != nil {
			Log(self.config_obj, collection_context,
				fmt.Sprintf("LogMessage: %v", err))
		}
		return err
	}

	if job.SessionId == constants.MONITORING_WELL_KNOWN_FLOW {
		err := MonitoringProcessMessage(
			ctx, self.config_obj, collection_context, job)
		if err != nil {
			Log(self.config_obj, collection_context,
				fmt.Sprintf("MonitoringProcessMessage: %v", err))
		}
		return err
	}

	err := ArtifactCollectorProcessOneMessage(ctx,
		self.config_obj, collection_context, job)
	if err != nil {
		Log(self.config_obj, collection_context,
			fmt.Sprintf("While processing job %v", err))
	}
	return err
}

func (self *FlowRunner) ProcessMessages(ctx context.Context,
	message_info *crypto.MessageInfo) error {

	self.mu.Lock()
	defer self.mu.Unlock()

	// Do some housekeeping with the client
	err := CheckClientStatus(ctx, self.config_obj, message_info.Source)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("ForemanCheckin for client %v: %v", message_info.Source, err)
	}

	return message_info.IterateJobs(ctx, self.config_obj, self.ProcessSingleMessage)
}
