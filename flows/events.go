package flows

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	config "www.velocidex.com/golang/velociraptor/config"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

type MonitoringFlow struct {
	*BaseFlow
}

func (self *MonitoringFlow) New() Flow {
	return &MonitoringFlow{&BaseFlow{}}
}

func (self *MonitoringFlow) Start(
	config_obj *config.Config,
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
	config_obj *config.Config,
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

		// Store the event log in the client's VFS.
		if response.Query.Name != "" {
			file_store_factory := file_store.GetFileStore(config_obj)

			now := time.Now()
			log_path := path.Join(
				"clients", flow_obj.RunnerArgs.ClientId,
				"monitoring", response.Query.Name,
				fmt.Sprintf("%d-%02d-%02d", now.Year(),
					now.Month(), now.Day()))
			fd, err := file_store_factory.WriteFile(log_path)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return err
			}
			defer fd.Close()

			// Seek to the end of the file.
			length, err := fd.Seek(0, os.SEEK_END)
			w := csv.NewWriter(fd)
			defer w.Flush()

			// A new file, write the headings.
			if err == nil && length == 0 {
				w.Write(response.Columns)
			}

			var rows []map[string]json.RawMessage
			err = json.Unmarshal([]byte(response.Response), &rows)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, row := range rows {
				csv_row := []string{}

				for _, column := range response.Columns {
					item, pres := row[column]
					if !pres {
						csv_row = append(csv_row, "-")
					} else {
						csv_row = append(csv_row, string(item))
					}
				}

				w.Write(csv_row)
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
