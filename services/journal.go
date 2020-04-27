// The journal service receives events from various sources and writes
// them to storage. Velociraptor uses the artifact name and source as
// the name of the queue that will be written.

// The service will also allow for registration of interested events.

// We use the underlying file store's queue manager to actually manage
// the notifications and watching and write the events to storage.

package services

import (
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
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

func (self *JournalService) PushRows(
	queue_name, flow_id, sender string,
	rows []*ordereddict.Dict) error {

	path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, source /* client_id */, flow_id, queue_name)
	return self.qm.PushEventRows(path_manager, sender, rows)
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
	qm, err := file_store.GetQueueManager(config_obj)
	if err != nil {
		return err
	}

	service := &JournalService{
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.FrontendComponent),
		qm:         qm,
	}

	return service.Start()
}
