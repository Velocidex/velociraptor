package flows

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	errors "github.com/pkg/errors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/responder"
	urns "www.velocidex.com/golang/velociraptor/urns"
)

var (
	gJournalWriter = NewJournalWriter()
)

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
		for {
			event, ok := <-result.Channel
			if !ok {
				return
			}
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
		columns := append([]string{"ClientId"}, event.Columns...)
		w.Write(columns)
	}

	var rows []map[string]interface{}
	err = json.Unmarshal([]byte(event.Response), &rows)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, row := range rows {
		csv_row := []interface{}{event.ClientId}

		for _, column := range event.Columns {
			item, pres := row[column]
			if !pres {
				csv_row = append(csv_row, "-")
			} else {
				csv_row = append(csv_row, item)
			}
		}
		w.WriteAny(csv_row)
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

		// Write the response on the journal.
		gJournalWriter.Channel <- &Event{
			Config:    config_obj,
			Timestamp: time.Now(),
			ClientId:  flow_obj.RunnerArgs.ClientId,
			QueryName: response.Query.Name,
			Response:  response.Response,
			Columns:   response.Columns,
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

			var rows []map[string]interface{}
			err = json.Unmarshal([]byte(response.Response), &rows)
			if err != nil {
				return errors.WithStack(err)
			}

			for _, row := range rows {
				csv_row := []interface{}{}

				for _, column := range response.Columns {
					item, pres := row[column]
					if !pres {
						csv_row = append(csv_row, "-")
					} else {
						csv_row = append(csv_row, item)
					}
				}

				w.WriteAny(csv_row)
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
