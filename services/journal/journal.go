// The journal service receives events from various sources and writes
// them to storage. Velociraptor uses the artifact name and source as
// the name of the queue that will be written.

// The service will also allow for registration of interested events.

// We use the underlying file store's queue manager to actually manage
// the notifications and watching and write the events to storage.

package journal

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
)

type JournalService struct {
	qm api.QueueManager

	logger     *logging.LogContext
	config_obj *config_proto.Config
}

func (self *JournalService) Watch(queue_name string) (
	output <-chan *ordereddict.Dict, cancel func()) {

	if self == nil || self.qm == nil {
		// Readers block on nil channel.
		return nil, func() {}
	}

	return self.qm.Watch(queue_name)
}

func (self *JournalService) PushRowsToArtifact(
	rows []*ordereddict.Dict, artifact string) error {

	path_manager := result_sets.NewArtifactPathManager(
		self.config_obj, "", "", artifact)

	return self.PushRows(path_manager, rows)
}

func (self *JournalService) PushRows(
	path_manager api.PathManager, rows []*ordereddict.Dict) error {
	if self != nil && self.qm != nil {
		return self.qm.PushEventRows(path_manager, rows)
	}
	return errors.New("Filestore not initialized")
}

func (self *JournalService) Start() error {
	self.logger.Info("<green>Starting</> Journal service.")
	return nil
}

func StartJournalService(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) error {
	qm, err := file_store.GetQueueManager(config_obj)
	if err != nil {
		return err
	}

	service := &JournalService{
		config_obj: config_obj,
		logger:     logging.GetLogger(config_obj, &logging.FrontendComponent),
		qm:         qm,
	}

	services.RegisterJournal(service)

	return service.Start()
}
