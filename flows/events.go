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
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	gJournalWriter = NewJournalWriter()

	gEventTable = &VQLEventTable{}
)

type VQLEventTable struct {
	mu               sync.Mutex
	flow_runner_args *flows_proto.FlowRunnerArgs
	CompressionDict  *flows_proto.ArtifactCompressionDict
	version          int64
}

func (self *VQLEventTable) GetFlowRunnerArgs(
	config_obj *api_proto.Config) (flows_proto.FlowRunnerArgs, error) {
	result := flows_proto.FlowRunnerArgs{}

	self.mu.Lock()
	defer self.mu.Unlock()

	// Create the runner args for the first time. In the current
	// implementation the server needs to be restarted before the
	// event table can be updated because the event table is
	// stored in the config file. Therefore this will only happen
	// the first time, and all other requests can use the same
	// flow runner args.
	if self.flow_runner_args == nil {
		self.CompressionDict = &flows_proto.ArtifactCompressionDict{}

		repository, err := artifacts.GetGlobalRepository(config_obj)
		if err != nil {
			return result, err
		}

		event_table := &actions_proto.VQLEventTable{
			Version: config_obj.Events.Version,
		}
		for _, name := range config_obj.Events.Artifacts {
			rate := config_obj.Events.OpsPerSecond
			if rate == 0 {
				rate = 100
			}

			vql_collector_args := &actions_proto.VQLCollectorArgs{
				MaxWait:      100,
				OpsPerSecond: rate,
			}
			artifact, pres := repository.Get(name)
			if !pres {
				return result, errors.New("Unknown artifact " + name)
			}

			err := repository.Compile(artifact, vql_collector_args)
			if err != nil {
				return result, err
			}
			// Add any artifact dependencies.
			repository.PopulateArtifactsVQLCollectorArgs(vql_collector_args)
			event_table.Event = append(event_table.Event, vql_collector_args)

			// Compress the VQL on the way out.
			if config_obj.Frontend.DoNotCompressArtifacts {
				continue
			}

			err = artifactCompress(vql_collector_args, self.CompressionDict)
			if err != nil {
				return result, err
			}
		}

		self.flow_runner_args = &flows_proto.FlowRunnerArgs{
			FlowName: "MonitoringFlow",
		}
		flow_args, err := ptypes.MarshalAny(event_table)
		if err != nil {
			return result, errors.WithStack(err)
		}
		self.flow_runner_args.Args = flow_args
	}

	// Return a copy to keep the flow_runner_args immutable
	return *self.flow_runner_args, nil
}

// What we write in the journal.
type Event struct {
	Config    *api_proto.Config
	Timestamp time.Time
	ClientId  string
	QueryName string
	Response  string
	Columns   []string
}

// The journal is a common CSV file which collects events from all
// clients. It is not meant to be kept for a long time so we dont care
// how large it grows. The main purpose for the journal is to be able
// to see all events from all clients at the same time. Server side
// VQL queries can watch the journal to determine when an event occurs
// on any client.
//
// Note that we write both the journal and the per client monitoring
// log.
//
// TODO: Add the pid into the journal filename to ensure that multiple
// writer processes dont collide. Within the same process there is a
// global writer so it can be used asyncronously.
type JournalWriter struct {
	Channel chan *Event
}

func NewJournalWriter() *JournalWriter {
	result := &JournalWriter{
		Channel: make(chan *Event, 10),
	}

	go func() {
		for event := range result.Channel {
			result.WriteEvent(event)
		}
	}()

	return result
}

func (self *JournalWriter) WriteEvent(event *Event) error {
	file_store_factory := file_store.GetFileStore(event.Config)

	now := time.Now()
	log_path := path.Join(
		"journals", event.QueryName,
		fmt.Sprintf("%d-%02d-%02d.csv", now.Year(),
			now.Month(), now.Day()))

	fd, err := file_store_factory.WriteFile(log_path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	defer fd.Close()

	scope := vql_subsystem.MakeScope()
	defer scope.Close()

	writer, err := csv.GetCSVWriter(scope, fd)
	if err != nil {
		return err
	}
	defer writer.Close()

	// Decode the VQLResponse and write into the CSV file.
	var rows []map[string]interface{}
	err = json.Unmarshal([]byte(event.Response), &rows)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, row := range rows {
		csv_row := vfilter.NewDict().
			Set("ClientId", event.ClientId)

		for _, column := range event.Columns {
			csv_row.Set(column, row[column])
		}
		writer.Write(csv_row)
	}

	return nil
}

type MonitoringFlow struct {
	*BaseFlow
}

func (self *MonitoringFlow) New() Flow {
	return &MonitoringFlow{&BaseFlow{}}
}

func (self *MonitoringFlow) Start(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	args proto.Message) error {

	flow_obj.Urn = urns.BuildURN(
		"clients", flow_obj.RunnerArgs.ClientId,
		"flows", constants.MONITORING_WELL_KNOWN_FLOW)

	event_table, ok := args.(*actions_proto.VQLEventTable)
	if !ok {
		return errors.New("Expected args of type VQLEventTable")
	}

	state := flow_obj.GetState()
	if state == nil {
		state = &flows_proto.ClientMonitoringState{}
	}

	return QueueMessageForClient(
		config_obj, flow_obj,
		"UpdateEventTable",
		event_table, processVQLResponses)
}

func (self *MonitoringFlow) ProcessMessage(
	config_obj *api_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendDataToFile(
			config_obj, flow_obj,
			path.Join("clients",
				flow_obj.RunnerArgs.ClientId,
				"uploads",
				path.Base(message.SessionId)),
			message)

	case processVQLResponses:
		payload := responder.ExtractGrrMessagePayload(message)
		if payload == nil {
			return nil
		}

		response, ok := payload.(*actions_proto.VQLResponse)
		if !ok {
			return nil
		}

		if !config_obj.Frontend.DoNotCompressArtifacts {
			artifactUncompress(response, gEventTable.CompressionDict)
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

			now := time.Now()
			log_path := path.Join(
				"clients", message.Source,
				"monitoring", response.Query.Name,
				fmt.Sprintf("%d-%02d-%02d.csv", now.Year(),
					now.Month(), now.Day()))
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
				csv_row := vfilter.NewDict()
				for _, column := range response.Columns {
					csv_row.Set(column, row[column])
				}

				writer.Write(csv_row)
			}
		}
	}
	return nil
}

func init() {
	default_args, _ := ptypes.MarshalAny(&actions_proto.VQLEventTable{})
	RegisterImplementation(&flows_proto.FlowDescriptor{
		Name:         "MonitoringFlow",
		FriendlyName: "Manage the client's Event monitoring table",
		Category:     "System",
		ArgsType:     "VQLEventTable",
		DefaultArgs:  default_args,
	}, &MonitoringFlow{})
}
