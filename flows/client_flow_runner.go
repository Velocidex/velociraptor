package flows

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/go-errors/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
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
	"www.velocidex.com/golang/velociraptor/services/launcher"
	utils "www.velocidex.com/golang/velociraptor/utils"
)

/*
  This flow runner processes messages from newer clients which support
  CLIENT_API_VERSION >= 4.

  These clients maintain the state of the flow on the client
  itself. This means the server does not need to maintain a lot of
  information about each flow making it much faster.

*/

type ClientFlowRunner struct {
	ctx        context.Context
	config_obj *config_proto.Config

	// The completer keeps track of all asynchronous filesystem
	// operations that will occur so that when everything is written
	// to disk, the completer can send the System.Flow.Completion
	// event. This is important as we dont want watchers of
	// System.Flow.Completion to attempt to open the collection before
	// everything is written.
	completer *utils.Completer
	closer    func()

	// If the flow is complete we send a completion message to the
	// master.
	flow_completion_messages []*ordereddict.Dict

	upload_completion_messages []*ordereddict.Dict
}

func NewFlowRunner(
	ctx context.Context,
	config_obj *config_proto.Config) *ClientFlowRunner {
	result := &ClientFlowRunner{
		ctx:        ctx,
		config_obj: config_obj,
	}

	// Wait for completion until Close() is called.
	result.completer = utils.NewCompleter(result.Complete)
	result.closer = result.completer.GetCompletionFunc()
	return result
}

// Delay sending events to the master node until all commits are
// complete and data becomes visible.
func (self *ClientFlowRunner) Complete() {
	if len(self.flow_completion_messages) > 0 {
		journal, err := services.GetJournal(self.config_obj)
		if err != nil {
			return
		}

		for _, row := range self.flow_completion_messages {
			journal.PushRowsToArtifactAsync(self.ctx,
				self.config_obj, row, "System.Flow.Completion")
		}
	}

	if len(self.upload_completion_messages) > 0 {
		journal, err := services.GetJournal(self.config_obj)
		if err != nil {
			return
		}

		for _, row := range self.upload_completion_messages {
			journal.PushRowsToArtifactAsync(self.ctx, self.config_obj,
				row, "System.Upload.Completion")
		}
	}
}

func (self *ClientFlowRunner) ProcessMonitoringMessage(
	ctx context.Context, msg *crypto_proto.VeloMessage) error {

	flow_id := msg.SessionId
	client_id := msg.Source

	if msg.VQLResponse != nil && msg.VQLResponse.Query != nil {
		err := self.MonitoringVQLResponse(ctx, client_id, flow_id, msg.VQLResponse)
		if err != nil {
			return fmt.Errorf("MonitoringVQLResponse: %w", err)
		}
		return self.maybeProcessClientInfo(ctx, client_id, msg.VQLResponse)
	}

	if msg.LogMessage != nil {
		err := self.MonitoringLogMessage(ctx, client_id, flow_id, msg.LogMessage)
		if err != nil {
			return fmt.Errorf("MonitoringLogMessage: %w", err)
		}
		return nil
	}

	if msg.FileBuffer != nil {
		err := self.FileBuffer(ctx, client_id, flow_id, msg.FileBuffer)
		if err != nil {
			return fmt.Errorf("FileBuffer: %w", err)
		}
		return nil
	}

	return nil
}

func (self *ClientFlowRunner) MonitoringLogMessage(
	ctx context.Context, client_id, flow_id string,
	response *crypto_proto.LogMessage) error {

	artifact_name := artifacts.DeobfuscateString(
		self.config_obj, response.Artifact)

	// If we are not able to deobfuscate the artifact name properly
	// (e.g. due to server keys changing) we really can not store the
	// data anywhere so drop it.
	if artifact_name == "" ||
		strings.HasPrefix(artifact_name, "$") {
		return nil
	}

	log_path_manager, err := artifact_paths.NewArtifactLogPathManager(ctx,
		self.config_obj, client_id, flow_id, artifact_name)
	if err != nil {
		return err
	}

	// Write the logs asynchronously
	rs_writer, err := result_sets.NewTimedResultSetWriter(
		self.config_obj, log_path_manager, json.DefaultEncOpts(),
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	// The JSON payload from the client.
	payload := artifacts.DeobfuscateString(self.config_obj, response.Jsonl)

	rs_writer.WriteJSONL([]byte(payload), int(response.NumberOfRows))

	if response.Level == logging.ALERT {
		return self.processMonitoringAlert(ctx, client_id, artifact_name, response)
	}

	return nil
}

func (self *ClientFlowRunner) processMonitoringAlert(
	ctx context.Context, client_id, artifact string,
	msg *crypto_proto.LogMessage) error {

	tmp := &log_message{}
	err := json.Unmarshal([]byte(msg.Jsonl), tmp)
	if err != nil {
		return err
	}

	alert := &services.AlertMessage{}
	err = json.Unmarshal([]byte(tmp.Message), alert)
	if err != nil {
		return err
	}

	alert.ClientId = client_id
	alert.Artifact = artifact
	alert.ArtifactType = "CLIENT_EVENT"

	serialized, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}
	return journal.PushJsonlToArtifact(ctx, self.config_obj,
		serialized, 1, "Server.Internal.Alerts", "server", "")
}

func (self *ClientFlowRunner) MonitoringVQLResponse(
	ctx context.Context, client_id, flow_id string,
	response *actions_proto.VQLResponse) error {

	// Ignore empty responses
	if response == nil || response.Query == nil ||
		response.Query.Name == "" || response.JSONLResponse == "" {
		return nil
	}

	// Deobfuscate the response if needed.
	_ = artifacts.Deobfuscate(self.config_obj, response)

	// If we are not able to deobfuscate the artifact name properly
	// (e.g. due to server keys changing) we really can not store the
	// data anywhere so drop it.
	query_name := response.Query.Name
	if query_name == "" || strings.HasPrefix(query_name, "$") {
		return nil
	}

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	// Append the client id to the data so we can see where it came
	// from.
	data := json.AppendJsonlItem(
		[]byte(response.JSONLResponse), "ClientId", client_id)

	return journal.PushJsonlToArtifact(ctx,
		self.config_obj, data, int(response.TotalRows),
		query_name, client_id, flow_id)
}

func (self *ClientFlowRunner) removeInflightChecks(
	ctx context.Context, client_id string) error {
	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}

	journal.PushRowsToArtifactAsync(ctx, self.config_obj,
		ordereddict.NewDict().
			Set("ClientId", client_id).
			Set("ClearFlows", true),
		"Server.Internal.ClientScheduled")

	// Update the client's in flight flow tracker on the local system
	// as well. This helps to update this record ASAP before waiting
	// for the minion message to arrive.
	client_info_manager, err := services.GetClientInfoManager(self.config_obj)
	if err != nil {
		return err
	}

	return client_info_manager.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
				client_info.ClientId = client_id
			}

			client_info.InFlightFlows = nil
			return client_info, nil
		})
}

func (self *ClientFlowRunner) ProcessSingleMessage(
	ctx context.Context, msg *crypto_proto.VeloMessage) error {

	flow_id := msg.SessionId
	client_id := msg.Source

	if flow_id == constants.MONITORING_WELL_KNOWN_FLOW {
		return self.ProcessMonitoringMessage(ctx, msg)
	}

	// This response can only happen when an error occured to the flow
	// status request. This means this old client does not support the
	// new check. We remove all inflight checks.
	if flow_id == constants.STATUS_CHECK_WELL_KNOWN_FLOW {
		return self.removeInflightChecks(ctx, client_id)
	}

	// Should never happen because these are filled in from the crypto
	// envelope.
	if flow_id == "" || client_id == "" {
		return fmt.Errorf("Empty SessionId: %v", msg)
	}

	if msg.LogMessage != nil {
		err := self.LogMessage(ctx, client_id, flow_id, msg.LogMessage)
		if err != nil {
			return fmt.Errorf("LogMessage: %w", err)
		}
		return nil
	}

	if msg.VQLResponse != nil {
		err := self.VQLResponse(ctx, client_id, flow_id, msg.VQLResponse)
		if err != nil {
			return fmt.Errorf("VQLResponse: %w", err)
		}
		return nil
	}

	if msg.FlowStats != nil {
		err := self.FlowStats(ctx, client_id, flow_id, msg.FlowStats)
		if err != nil {
			return fmt.Errorf("FlowStats: %w", err)
		}
		return nil
	}

	if msg.FileBuffer != nil {
		err := self.FileBuffer(ctx, client_id, flow_id, msg.FileBuffer)
		if err != nil {
			return fmt.Errorf("FileBuffer: %w", err)
		}
		return nil
	}

	if msg.UploadTransaction != nil {
		err := self.UploadTransaction(ctx, client_id, flow_id, msg.UploadTransaction)
		if err != nil {
			return fmt.Errorf("UploadTransaction: %w", err)
		}
		return nil
	}

	return nil
}

func (self *ClientFlowRunner) FileBuffer(
	ctx context.Context, client_id, flow_id string,
	file_buffer *actions_proto.FileBuffer) error {

	if file_buffer == nil || file_buffer.Pathspec == nil {
		return errors.New("Expected args of type FileBuffer")
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)

	// Figure out where to store the file.
	file_path_manager := flow_path_manager.GetUploadsFile(
		file_buffer.Pathspec.Accessor, file_buffer.Pathspec.Path,
		file_buffer.Pathspec.Components)

	fd, err := file_store_factory.WriteFile(file_path_manager.Path())
	if err != nil {
		// If we fail to write this one file we keep going -
		// otherwise the flow will be terminated.
		logger.Error("While writing to %v: %v",
			file_path_manager.Path().AsClientPath(), err)
		return nil
	}
	defer fd.Close()

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {

		// Truncate the file on first buffer received.
		err = fd.Truncate()
		if err != nil {
			return err
		}
	}

	if file_buffer.UncompressedLength > 0 {
		_, err = fd.WriteCompressed(file_buffer.Data,
			file_buffer.Offset,
			int(file_buffer.UncompressedLength))

	} else {
		// Write the actual data to the file.
		_, err = fd.Write(file_buffer.Data)
	}

	if err != nil {
		logger.Error("While writing to %v: %v",
			file_path_manager.Path().AsClientPath(), err)
		return nil
	}

	// The EOF status represents the file is complete - do some housework
	if file_buffer.Eof {
		uploadCounter.Inc()
		uploadBytes.Add(float64(file_buffer.StoredSize))

		// When the upload completes, we emit an event.
		self.upload_completion_messages = append(
			self.upload_completion_messages,
			ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().Unix()).
				Set("ClientId", client_id).
				Set("VFSPath", file_path_manager.Path().AsClientPath()).
				Set("UploadName", file_buffer.Pathspec.Path).
				Set("Accessor", file_buffer.Pathspec.Accessor).
				Set("Size", file_buffer.Size).
				Set("UploadedSize", file_buffer.StoredSize))

		// Write the upload to the uplod metadata
		rs_writer, err := result_sets.NewResultSetWriter(
			file_store_factory, flow_path_manager.UploadMetadata(),
			json.DefaultEncOpts(),
			self.completer.GetCompletionFunc(),
			result_sets.AppendMode)
		if err != nil {
			return err
		}

		defer rs_writer.Close()

		rs_writer.Write(ordereddict.NewDict().
			Set("Timestamp", utils.GetTime().Now().UTC().Unix()).
			Set("started", utils.GetTime().Now().UTC().String()).
			Set("vfs_path", file_path_manager.VisibleVFSPath()).
			Set("Type", "").
			Set("_Components", file_path_manager.Path().Components()).
			Set("file_size", file_buffer.Size).

			// The client's components and accessor that were used to
			// upload the file.
			Set("_accessor", file_buffer.Pathspec.Accessor).
			Set("_client_components", file_buffer.Pathspec.Components).
			Set("uploaded_size", file_buffer.StoredSize))

		// Additional row for sparse files
		if file_buffer.Index != nil {
			rs_writer.Write(ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().Unix()).
				Set("started", time.Now().UTC().String()).
				Set("vfs_path", file_path_manager.VisibleVFSPath()+".idx").
				Set("Type", "idx").
				Set("_Components", file_path_manager.Path().Components()).
				Set("_accessor", file_buffer.Pathspec.Accessor).
				Set("_client_components", file_buffer.Pathspec.Components).
				Set("file_size", file_buffer.Size).
				Set("uploaded_size", file_buffer.StoredSize))

			// Write the index file (NOTE: the FileBuffer.Index only
			// appears at the list Eof Packet.
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
		}
	}

	return nil
}

func (self *ClientFlowRunner) UploadTransaction(
	ctx context.Context, client_id, flow_id string,
	transaction *actions_proto.UploadTransaction) error {

	file_store_factory := file_store.GetFileStore(self.config_obj)

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)

	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, flow_path_manager.UploadTransactions(),
		json.DefaultEncOpts(),
		self.completer.GetCompletionFunc(),
		result_sets.AppendMode)
	if err != nil {
		return err
	}

	defer rs_writer.Close()

	rs_writer.Write(json.ConvertProtoToOrderedDict(transaction))

	return nil
}

func (self *ClientFlowRunner) Close(ctx context.Context) {
	self.closer()
}

// If the client does not know about the flow, we need to terminate
// the existing flow. This is a bit slower as we need to open the flow
// for reading.
func (self *ClientFlowRunner) handleUnknwonFlow(
	ctx context.Context, client_id, flow_id string,
	msg *crypto_proto.FlowStats) error {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Just a blind write will eventually hit the disk.
	stats := &flows_proto.ArtifactCollectorContext{}
	err = db.GetSubject(self.config_obj, flow_path_manager.Stats(), stats)
	if err != nil {
		return nil
	}

	// Mark all the stats as terminated if they are still running.
	for _, s := range stats.QueryStats {
		if s.Status == crypto_proto.VeloStatus_PROGRESS {
			s.Status = crypto_proto.VeloStatus_GENERIC_ERROR
			s.ErrorMessage = msg.QueryStatus[0].ErrorMessage
		}
	}

	launcher.UpdateFlowStats(stats)

	return db.SetSubjectWithCompletion(self.config_obj,
		flow_path_manager.Stats(), stats, nil)
}

func (self *ClientFlowRunner) FlowStats(
	ctx context.Context, client_id, flow_id string,
	msg *crypto_proto.FlowStats) error {

	if msg.FlowComplete && len(msg.QueryStatus) == 1 {
		stats := msg.QueryStatus[0]
		if stats.Status == crypto_proto.VeloStatus_UNKNOWN_FLOW ||

			// Backwards compatibility
			strings.HasPrefix(stats.ErrorMessage, "Flow not known") {
			return self.handleUnknwonFlow(ctx, client_id, flow_id, msg)
		}
	}

	// Write a partial ArtifactCollectorContext protobuf containing
	// all the dynamic fields
	stats := &flows_proto.ArtifactCollectorContext{
		ClientId:                   client_id,
		SessionId:                  flow_id,
		TotalUploadedFiles:         msg.TotalUploadedFiles,
		TotalExpectedUploadedBytes: msg.TotalExpectedUploadedBytes,
		TotalUploadedBytes:         msg.TotalUploadedBytes,
		TotalCollectedRows:         msg.TotalCollectedRows,
		TransactionsOutstanding:    msg.TransactionsOutstanding,
		TotalLogs:                  msg.TotalLogs,
		ActiveTime:                 msg.Timestamp,
		QueryStats:                 msg.QueryStatus,
	}

	// Deobfuscate artifact names
	for _, s := range stats.QueryStats {
		if len(s.NamesWithResponse) > 0 {
			s.NamesWithResponse = deobfuscateNames(
				self.config_obj, s.NamesWithResponse)
		}
	}

	// Recompose the flow context from the QueryStats
	launcher.UpdateFlowStats(stats)

	// Store the updated flow object in the datastore
	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id)
	db, err := datastore.GetDB(self.config_obj)
	if err != nil {
		return err
	}

	// Just a blind write will eventually hit the disk.
	err = db.SetSubjectWithCompletion(self.config_obj,
		flow_path_manager.Stats(), stats, nil)
	if err != nil {
		return err
	}

	// Update the client's in flight flow tracker.
	client_info_manager, err := services.GetClientInfoManager(self.config_obj)
	if err != nil {
		return err
	}

	// If this is the final response, then we will notify a flow
	// completion.
	if msg.FlowComplete {
		// Immediately remove this flow from the local InFlightFlows
		// so we dont schedule it again.
		err := client_info_manager.Modify(ctx, client_id,
			func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
				if client_info == nil {
					client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
					client_info.ClientId = client_id
				}

				if client_info.InFlightFlows == nil {
					client_info.InFlightFlows = make(map[string]int64)
				}

				_, pres := client_info.InFlightFlows[flow_id]
				if !pres {
					return client_info, utils.NotFoundError
				}

				// Update the timestamp that we last received a stats
				// update from this flow.
				delete(client_info.InFlightFlows, flow_id)
				return client_info, nil
			})

		// We have no idea about this flow - we need to check more closely.
		if utils.IsNotFound(err) {
			// Slow path - normally the flow will already be in the
			// InFlightFlows so most of the time this wont
			// happen. However, if the flow is unexpected we need to
			// load the collection from the datastore and check it
			// more closely.
			launcher, err := services.GetLauncher(self.config_obj)
			if err != nil {
				return err
			}

			flow_context, err := launcher.Storage().LoadCollectionContext(
				ctx, self.config_obj, client_id, flow_id)
			if err != nil {
				return err
			}

			// If the flow is already done, do not send the Completion
			// message again!
			if flow_context.State == flows_proto.ArtifactCollectorContext_ERROR ||
				flow_context.State == flows_proto.ArtifactCollectorContext_FINISHED {
				return nil
			}
		}

		self.flow_completion_messages = append(self.flow_completion_messages,
			ordereddict.NewDict().
				Set("Timestamp", time.Now().UTC().Unix()).
				Set("Flow", stats).
				Set("FlowId", flow_id).
				Set("ClientId", client_id))

		return err
	}

	return client_info_manager.Modify(ctx, client_id,
		func(client_info *services.ClientInfo) (*services.ClientInfo, error) {
			if client_info == nil {
				client_info = &services.ClientInfo{ClientInfo: &actions_proto.ClientInfo{}}
				client_info.ClientId = client_id
			}

			if client_info.InFlightFlows == nil {
				client_info.InFlightFlows = make(map[string]int64)
			}

			// Update the timestamp that we last received a stats
			// update from this flow.
			client_info.InFlightFlows[flow_id] = utils.GetTime().Now().Unix()
			return client_info, nil
		})
}

func (self *ClientFlowRunner) VQLResponse(
	ctx context.Context, client_id, flow_id string,
	response *actions_proto.VQLResponse) error {

	if response == nil || response.Query == nil || response.Query.Name == "" {
		return nil
	}

	err := artifacts.Deobfuscate(self.config_obj, response)
	if err != nil {
		return err
	}

	if response.Query.Name == "" ||
		strings.HasPrefix(response.Query.Name, "$") {
		return nil
	}

	path_manager, err := artifact_paths.NewArtifactPathManager(ctx,
		self.config_obj, client_id, flow_id, response.Query.Name)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.Path(), json.DefaultEncOpts(),
		self.completer.GetCompletionFunc(),
		result_sets.AppendMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	// Old clients do not have QueryStartRow so it will be 0:
	if response.Part > 0 && response.QueryStartRow == 0 {

		// Exclusion for older clients without QueryStartRow: The
		// first packet will be QueryStartRow = 0, Part = 0, Existing
		// row = 0. If the first packet is retransmitted we can detect
		// it. For old clients, the next packet will be QueryStartRow
		// = 0 but Part > 0 - which means we must ignore QueryStartRow
		// and NOT set the start row.

	} else {
		// Modern clients set the start row properly. We can check it
		// against the result set index to ensure these rows are
		// appended at the correct index in the result set.
		err := rs_writer.SetStartRow(int64(response.QueryStartRow))
		if err != nil {
			// Something is wrong - the packet may have been
			// retransmitted - Just drop it on the floor.
			return err
		}
	}

	if response.UncompressedSize > 0 {
		rs_writer.WriteCompressedJSONL(
			response.CompressedJsonResponse,
			response.ByteOffset, int(response.UncompressedSize),
			response.TotalRows)

	} else {
		rs_writer.WriteJSONL(
			[]byte(response.JSONLResponse), response.TotalRows)
	}

	return nil
}

type log_message struct {
	Message string `json:"message"`
}

func (self *ClientFlowRunner) processAlert(
	ctx context.Context, client_id, flow_id string,
	msg *crypto_proto.LogMessage) error {

	tmp := &log_message{}
	err := json.Unmarshal([]byte(msg.Jsonl), tmp)
	if err != nil {
		return err
	}

	alert := &services.AlertMessage{}
	err = json.Unmarshal([]byte(tmp.Message), alert)
	if err != nil {
		return err
	}

	alert.ClientId = client_id
	alert.FlowId = flow_id

	serialized, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')

	journal, err := services.GetJournal(self.config_obj)
	if err != nil {
		return err
	}
	return journal.PushJsonlToArtifact(ctx, self.config_obj,
		serialized, 1, "Server.Internal.Alerts", "server", "")
}

func (self *ClientFlowRunner) LogMessage(
	ctx context.Context, client_id, flow_id string,
	msg *crypto_proto.LogMessage) error {

	flow_path_manager := paths.NewFlowPathManager(client_id, flow_id).Log()

	// Append logs to messages from previous packets.
	file_store_factory := file_store.GetFileStore(self.config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, flow_path_manager,
		json.DefaultEncOpts(), self.completer.GetCompletionFunc(),
		result_sets.AppendMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	_ = rs_writer.SetStartRow(int64(msg.Id))

	// The JSON payload from the client.
	payload := artifacts.DeobfuscateString(self.config_obj, msg.Jsonl)

	rs_writer.WriteJSONL([]byte(payload), uint64(msg.NumberOfRows))

	if msg.Level == logging.ALERT {
		return self.processAlert(ctx, client_id, flow_id, msg)
	}

	return nil
}

func (self *ClientFlowRunner) ProcessMessages(ctx context.Context,
	message_info *crypto.MessageInfo) error {

	// Do some housekeeping with the client
	err := CheckClientStatus(ctx, self.config_obj, message_info.Source)
	if err != nil {
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.Error("ForemanCheckin for client %v: %v", message_info.Source, err)
	}

	return message_info.IterateJobs(ctx, self.config_obj, self.ProcessSingleMessage)
}
