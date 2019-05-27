package flows

import (
	"encoding/json"
	"fmt"
	"time"

	errors "github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

var (
	gJournalWriter = NewJournalWriter()
)

// What we write in the journal's channel.
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

	artifact_name, source_name := artifacts.
		QueryNameToArtifactAndSource(event.QueryName)

	log_path := artifacts.GetCSVPath(
		/* client_id */ "",
		artifacts.GetDayName(),
		/* flow_id */ "",
		artifact_name, source_name,
		artifacts.MODE_JOURNAL_DAILY)

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
			Set("_ts", int(time.Now().Unix())).
			Set("ClientId", event.ClientId)

		for _, column := range event.Columns {
			csv_row.Set(column, row[column])
		}
		writer.Write(csv_row)
	}

	return nil
}
