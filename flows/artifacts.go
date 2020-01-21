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
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	constants "www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/crypto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/datastore"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	utils "www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
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

const (
	_                          = iota
	processVQLResponses uint64 = iota
)

func NewFlowId(client_id string) string {
	buf := make([]byte, 8)
	rand.Read(buf)

	binary.BigEndian.PutUint32(buf, uint32(time.Now().Unix()))
	result := base32.HexEncoding.EncodeToString(buf)[:13]

	return constants.FLOW_PREFIX + result
}

func GetCollectionPath(client_id, flow_id string) string {
	return path.Join("/clients", client_id, "collections", flow_id)
}

func ScheduleArtifactCollection(
	config_obj *config_proto.Config,
	collector_request *flows_proto.ArtifactCollectorArgs) (string, error) {

	client_id := collector_request.ClientId
	if client_id == "" {
		return "", errors.New("Client id not provided.")
	}

	repository, err := artifacts.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	// Update the flow's artifacts list.
	vql_collector_args := &actions_proto.VQLCollectorArgs{
		OpsPerSecond: collector_request.OpsPerSecond,
		Timeout:      collector_request.Timeout,
		MaxRow:       1000,
	}
	for _, name := range collector_request.Artifacts {
		var artifact *artifacts_proto.Artifact = nil
		if collector_request.AllowCustomOverrides {
			artifact, _ = repository.Get("Custom." + name)
		}

		if artifact == nil {
			artifact, _ = repository.Get(name)
		}

		if artifact == nil {
			return "", errors.New("Unknown artifact " + name)
		}

		err := repository.Compile(artifact, vql_collector_args)
		if err != nil {
			return "", err
		}
	}

	// Add any artifact dependencies.
	err = repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
	if err != nil {
		return "", err
	}

	err = AddArtifactCollectorArgs(
		config_obj, vql_collector_args, collector_request)
	if err != nil {
		return "", err
	}

	err = artifacts.Obfuscate(config_obj, vql_collector_args)
	if err != nil {
		return "", err
	}

	// Generate a new collection context.
	collection_context := &flows_proto.ArtifactCollectorContext{
		SessionId:  NewFlowId(client_id),
		CreateTime: uint64(time.Now().UnixNano() / 1000),
		State:      flows_proto.ArtifactCollectorContext_RUNNING,
		Request:    collector_request,
	}
	collection_context.Urn = GetCollectionPath(client_id, collection_context.SessionId)

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return "", err
	}

	// Save the collection context.
	err = db.SetSubject(config_obj, collection_context.Urn, collection_context)
	if err != nil {
		return "", err
	}

	// The task we will schedule for the client.
	task := &crypto_proto.GrrMessage{
		SessionId:       collection_context.SessionId,
		RequestId:       processVQLResponses,
		VQLClientAction: vql_collector_args}

	// Record the tasks for provenance of what we actually did.
	err = db.SetSubject(config_obj,
		path.Join(collection_context.Urn, "task"),
		&api_proto.ApiFlowRequestDetails{
			Items: []*crypto_proto.GrrMessage{task}})
	if err != nil {
		return "", err
	}

	return collection_context.SessionId, QueueMessageForClient(
		config_obj, client_id, task)
}

func closeContext(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {
	if !collection_context.Dirty {
		return nil
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

	// This is the final time we update the context - send a
	// journal message.
	if collection_context.Request != nil &&
		collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("Flow", collection_context).
			Set("FlowId", collection_context.SessionId)
		serialized, err := json.Marshal([]vfilter.Row{row})
		if err != nil {
			return err
		}

		gJournalWriter.Channel <- &Event{
			Config:    config_obj,
			ClientId:  collection_context.Request.ClientId,
			QueryName: "System.Flow.Completion",
			Response:  string(serialized),
			Columns:   []string{"Timestamp", "Flow", "FlowId"},
		}
	}

	db, err := datastore.GetDB(config_obj)
	if err != nil {
		collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
		collection_context.Status = err.Error()
	}

	return db.SetSubject(config_obj, collection_context.Urn,
		collection_context)
}

func flushContextLogs(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext) error {
	log_path := path.Join(collection_context.Urn, "logs")

	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(log_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Seek to the end of the file.
	length, err := fd.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}

	w := csv.NewWriter(fd)
	defer w.Flush()

	headers_written := length > 0
	if !headers_written {
		w.Write([]string{"Timestamp", "time", "message"})
	}

	for _, row := range collection_context.Logs {
		w.Write([]string{
			fmt.Sprintf("%v", row.Timestamp),
			time.Unix(int64(row.Timestamp)/1000000, 0).String(),
			row.Message})
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

	log_path := path.Join(collection_context.Urn, "uploads.csv")
	file_store_factory := file_store.GetFileStore(config_obj)
	fd, err := file_store_factory.WriteFile(log_path)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Seek to the end of the file.
	length, err := fd.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}

	w := csv.NewWriter(fd)
	defer w.Flush()

	headers_written := length > 0
	if !headers_written {
		w.Write([]string{"Timestamp", "started", "vfs_path",
			"expected_size"})
	}

	for _, row := range collection_context.UploadedFiles {
		w.Write([]string{
			fmt.Sprintf("%v", time.Now().UTC().Unix()),
			time.Now().UTC().String(),
			row.Name,
			fmt.Sprintf("%v", row.Size)})
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
			Urn:       GetCollectionPath(client_id, flow_id),
			SessionId: flow_id,
		}, nil
	}

	urn := GetCollectionPath(client_id, flow_id)

	collection_context := &flows_proto.ArtifactCollectorContext{}
	db, err := datastore.GetDB(config_obj)
	if err != nil {
		return nil, err
	}

	err = db.GetSubject(config_obj, urn, collection_context)
	if err != nil {
		return nil, err
	}

	if collection_context.SessionId == "" {
		return nil, errors.New("Unknown flow " + client_id + " " + flow_id)
	}

	collection_context.Dirty = false
	collection_context.Urn = urn

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

		// Restore strings from flow state.
		response := message.VQLResponse
		if response == nil {
			return errors.New("Expected args of type VQLResponse")
		}

		err = artifacts.Deobfuscate(config_obj, response)
		if err != nil {
			return err
		}

		artifact_name, source_name := artifacts.
			QueryNameToArtifactAndSource(response.Query.Name)

		// Store the event log in the client's VFS.
		if response.Query.Name != "" {
			log_path := artifacts.GetCSVPath(
				collection_context.Request.ClientId, "",
				collection_context.SessionId,
				artifact_name, source_name, artifacts.MODE_CLIENT)

			file_store_factory := file_store.GetFileStore(config_obj)
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

			// Decode the JSON data.
			var rows []map[string]interface{}
			err = json.Unmarshal([]byte(response.Response), &rows)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, row := range rows {
				csv_row := ordereddict.NewDict()

				for _, column := range response.Columns {
					item, pres := row[column]
					if !pres {
						csv_row.Set(column, "-")
					} else {
						csv_row.Set(column, item)
					}
				}

				writer.Write(csv_row)
			}

			// Update the artifacts with results in the
			// context.
			if len(rows) > 0 && !utils.InString(
				&collection_context.ArtifactsWithResults,
				response.Query.Name) {
				collection_context.ArtifactsWithResults = append(
					collection_context.ArtifactsWithResults,
					response.Query.Name)
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

	if message.Status == nil {
		return false, nil
	}

	if constants.HuntIdRegex.MatchString(collection_context.Request.Creator) {
		err := services.GetHuntDispatcher().ModifyHunt(
			collection_context.Request.Creator,
			func(hunt *api_proto.Hunt) error {
				hunt.Stats.TotalClientsWithResults++
				return nil
			})
		if err != nil {
			return true, err
		}
	}

	// Only terminate a running flow.
	if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return true, nil
	}

	collection_context.State = flows_proto.ArtifactCollectorContext_TERMINATED
	collection_context.KillTimestamp = uint64(time.Now().UnixNano() / 1000)
	collection_context.Dirty = true

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

	// Only terminate a running flows.
	if collection_context.State != flows_proto.ArtifactCollectorContext_RUNNING {
		return errors.New(message.Status.ErrorMessage)
	}

	collection_context.State = flows_proto.ArtifactCollectorContext_ERROR
	collection_context.KillTimestamp = uint64(time.Now().UnixNano() / 1000)
	collection_context.Status = message.Status.ErrorMessage
	collection_context.Backtrace = message.Status.Backtrace
	collection_context.Dirty = true

	// Update the hunt stats if this is a hunt.
	if constants.HuntIdRegex.MatchString(collection_context.Request.Creator) {
		services.GetHuntDispatcher().ModifyHunt(
			collection_context.Request.Creator,
			func(hunt *api_proto.Hunt) error {
				hunt.Stats.TotalClientsWithErrors++
				return nil
			})
	}

	return errors.New(message.Status.ErrorMessage)
}

func appendUploadDataToFile(
	config_obj *config_proto.Config,
	collection_context *flows_proto.ArtifactCollectorContext,
	message *crypto_proto.GrrMessage) error {

	file_buffer := message.FileBuffer
	if file_buffer == nil {
		return errors.New("Expected args of type FileBuffer")
	}

	file_store_factory := file_store.GetFileStore(config_obj)

	// Figure out where to store the file.
	file_path := artifacts.GetUploadsFile(
		message.Source,
		collection_context.SessionId,
		file_buffer.Pathspec.Accessor,
		file_buffer.Pathspec.Path)

	fd, err := file_store_factory.WriteFile(file_path)
	if err != nil {
		// If we fail to write this one file we keep going -
		// otherwise the flow will be terminated.
		Log(config_obj, collection_context,
			fmt.Sprintf("While writing to %v: %v",
				file_path, err))
		return nil
	}
	defer fd.Close()

	// Keep track of all the files we uploaded.
	if file_buffer.Offset == 0 {
		fd.Truncate(0)
		collection_context.TotalUploadedFiles += 1
		collection_context.TotalExpectedUploadedBytes += file_buffer.Size
		collection_context.UploadedFiles = append(
			collection_context.UploadedFiles,
			&flows_proto.ArtifactUploadedFileInfo{
				Name: file_path,
				Size: file_buffer.Size,
			})
		collection_context.Dirty = true
	}

	if len(file_buffer.Data) > 0 {
		collection_context.TotalUploadedBytes += uint64(len(file_buffer.Data))
		collection_context.Dirty = true
	}

	fd.Seek(int64(file_buffer.Offset), 0)
	_, err = fd.Write(file_buffer.Data)
	if err != nil {
		Log(config_obj, collection_context,
			fmt.Sprintf("While writing to %v: %v", file_path, err))
		return nil
	}

	// When the upload completes, we emit an event.
	if file_buffer.Eof {
		uploadCounter.Inc()

		size := file_buffer.Offset + uint64(len(file_buffer.Data))
		uploadBytes.Add(float64(size))

		row := ordereddict.NewDict().
			Set("Timestamp", time.Now().UTC().Unix()).
			Set("ClientId", message.Source).
			Set("VFSPath", file_path).
			Set("UploadName", file_buffer.Pathspec.Path).
			Set("Accessor", file_buffer.Pathspec.Accessor).
			Set("Size", size)

		serialized, err := json.Marshal([]vfilter.Row{row})
		if err == nil {
			gJournalWriter.Channel <- &Event{
				Config:    config_obj,
				ClientId:  message.Source,
				QueryName: "System.Upload.Completion",
				Response:  string(serialized),
				Columns: []string{"Timestamp", "ClientId",
					"VFSPath", "UploadName",
					"Accessor", "Size"},
			}
		}
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
		closeContext(self.config_obj, collection_context)
	}
}

func (self *FlowRunner) ProcessSingleMessage(job *crypto_proto.GrrMessage) {
	if job.ForemanCheckin != nil {
		ForemanProcessMessage(
			self.config_obj, job.Source, job.ForemanCheckin)
		return
	}

	collection_context, pres := self.context_map[job.SessionId]
	if !pres {
		var err error

		collection_context, err = LoadCollectionContext(
			self.config_obj, job.Source, job.SessionId)
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error(fmt.Sprintf("Unable to load flow %s: %v", job.SessionId, err))
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

// Adds any parameters set in the ArtifactCollectorArgs into the
// VQLCollectorArgs.
func AddArtifactCollectorArgs(
	config_obj *config_proto.Config,
	vql_collector_args *actions_proto.VQLCollectorArgs,
	collector_args *flows_proto.ArtifactCollectorArgs) error {

	// Add any Environment Parameters from the request.
	if collector_args.Parameters == nil {
		return nil
	}

	for _, item := range collector_args.Parameters.Env {
		vql_collector_args.Env = append(vql_collector_args.Env,
			&actions_proto.VQLEnv{Key: item.Key, Value: item.Value})
	}

	return nil
}
