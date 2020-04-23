// The journal service receives events from various sources and writes
// them to storage. Velociraptor uses the artifact name and source as
// the name of the queue that will be written.

// The service will also allow for registration of interested events.

// We use the underlying file store's queue manager to actually manage
// the notifications and watching and write the events to storage.

package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

var (
	journal_mu sync.Mutex

	// Service is only available in the frontend.
	GJournal *JournalService = nil
)

type JournalService struct {
	qm api.QueueManager

	logger     *logging.LogContext
	config_obj *config_proto.Config
}

func (self *JournalService) Watch(queue_name string) (
	output <-chan *ordereddict.Dict, cancel func()) {
	return self.qm.Watch(queue_name)
}

func (self *JournalService) PushRow(queue_name, source string, mode int, row *ordereddict.Dict) error {
	err := self.qm.PushRow(queue_name, source, mode, row)
	if err != nil {
		return err
	}

	return self.write_rows(queue_name, source, mode, []*ordereddict.Dict{row})
}

func (self *JournalService) Push(queue_name, source string, mode int, rows []byte) error {
	err := self.qm.Push(queue_name, source, mode, rows)
	if err != nil {
		return err
	}

	dict_rows, err := utils.ParseJsonToDicts(rows)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}

	return self.write_rows(queue_name, source, mode, dict_rows)
}

func (self *JournalService) write_rows(queue_name, source string, mode int,
	dict_rows []*ordereddict.Dict) error {
	// Write the event into the client's monitoring log
	file_store_factory := file_store.GetFileStore(self.config_obj)
	artifact_name, source_name := paths.QueryNameToArtifactAndSource(queue_name)

	log_path := paths.GetCSVPath(
		source, /* client_id */
		paths.GetDayName(),
		"", artifact_name, source_name, mode)

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

	for _, row := range dict_rows {
		row.Set("_ts", int(time.Now().Unix()))
		writer.Write(row)
	}

	return nil
}

func (self *JournalService) Start() error {
	self.logger.Info("Starting Journal service.")
	journal_mu.Lock()
	defer journal_mu.Unlock()

	GJournal = self

	return nil
}

func GetJournal() *JournalService {
	journal_mu.Lock()
	defer journal_mu.Unlock()

	return GJournal
}

func StartJournalService(config_obj *config_proto.Config) error {
	service := &JournalService{
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.FrontendComponent),
		qm:         file_store.GetQueueManager(config_obj),
	}

	return service.Start()
}
