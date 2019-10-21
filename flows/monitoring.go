package flows

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	urns "www.velocidex.com/golang/velociraptor/urns"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type MonitoringFlow struct {
	*BaseFlow
}

func (self *MonitoringFlow) New() Flow {
	return &MonitoringFlow{&BaseFlow{}}
}

func (self *MonitoringFlow) Start(
	config_obj *config_proto.Config,
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
		config_obj, flow_obj.RunnerArgs.ClientId,
		&crypto_proto.GrrMessage{
			SessionId:        flow_obj.Urn,
			RequestId:        processVQLResponses,
			UpdateEventTable: event_table})
}

func (self *MonitoringFlow) ProcessMessage(
	config_obj *config_proto.Config,
	flow_obj *AFF4FlowObject,
	message *crypto_proto.GrrMessage) error {

	err := flow_obj.FailIfError(config_obj, message)
	if err != nil {
		return err
	}

	flow_obj.RunnerArgs.ClientId = message.Source

	switch message.RequestId {
	case constants.TransferWellKnownFlowId:
		return appendDataToFile(config_obj, flow_obj, message)

	case processVQLResponses:
		response := message.VQLResponse
		if response == nil {
			return nil
		}

		// Deobfuscate the response if needed.
		err := artifacts.Deobfuscate(config_obj, response)
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
				path.Base(flow_obj.Urn),
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
				csv_row := vfilter.NewDict().Set(
					"_ts", int(time.Now().Unix()))
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
