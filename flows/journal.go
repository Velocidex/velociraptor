package flows

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	errors "github.com/pkg/errors"
	artifacts "www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	gJournalWriter = NewJournalWriter()
)

// What we write in the journal's channel.
type Event struct {
	Config    *config_proto.Config
	Timestamp time.Time
	ClientId  string
	QueryName string
	Response  string
	Columns   []string
}

type Writer struct {
	*csv.CSVWriter

	closer func()
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

	// We keep a cache of writers to write the output of each
	// monitoring artifact. Note that each writer is responsible
	// for its own flushing etc.
	writers map[string]*Writer

	mu sync.Mutex
}

func NewJournalWriter() *JournalWriter {
	result := &JournalWriter{
		Channel: make(chan *Event, 10),
		writers: make(map[string]*Writer),
	}

	go func() {
		for event := range result.Channel {
			result.WriteEvent(event)
		}
	}()

	go func() {
		// Every minute flush all the writers and close the
		// file. This allows the file to be rotated properly.
		for {
			result.mu.Lock()
			for _, writer := range result.writers {
				writer.closer()
			}

			result.writers = make(map[string]*Writer)
			result.mu.Unlock()

			time.Sleep(60 * time.Second)
		}
	}()

	return result
}

func (self *JournalWriter) WriteEvent(event *Event) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	artifact_name, source_name := artifacts.
		QueryNameToArtifactAndSource(event.QueryName)

	log_path := artifacts.GetCSVPath(
		/* client_id */ "",
		artifacts.GetDayName(),
		/* flow_id */ "",
		artifact_name, source_name,
		artifacts.MODE_JOURNAL_DAILY)

	// Fetch the CSV writer for this journal file
	writer, pres := self.writers[log_path]
	if !pres {
		file_store_factory := file_store.GetFileStore(event.Config)

		fd, err := file_store_factory.WriteFile(log_path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}

		scope := vql_subsystem.MakeScope()

		csv_writer, err := csv.GetCSVWriter(scope, fd)
		if err != nil {
			return err
		}

		writer = &Writer{
			csv_writer,
			func() {
				scope.Close()
				csv_writer.Close()
				fd.Close()
			}}

		self.writers[log_path] = writer
	}

	// Decode the VQLResponse and write into the CSV file.
	var rows []map[string]interface{}
	err := json.Unmarshal([]byte(event.Response), &rows)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, row := range rows {
		csv_row := ordereddict.NewDict().
			Set("_ts", int(time.Now().Unix())).
			Set("ClientId", event.ClientId)

		for _, column := range event.Columns {
			csv_row.Set(column, row[column])
		}
		writer.Write(csv_row)
	}

	return nil
}
