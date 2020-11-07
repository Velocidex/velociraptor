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
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/services"
)

type JournalService struct {
	qm api.QueueManager
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
	config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flows_id string) error {

	path_manager := artifacts.NewArtifactPathManager(
		config_obj, client_id, flows_id, artifact)
	return self.PushRows(config_obj, path_manager, rows)
}

func (self *JournalService) PushRows(
	config_obj *config_proto.Config,
	path_manager api.PathManager, rows []*ordereddict.Dict) error {
	if self != nil && self.qm != nil {
		return self.qm.PushEventRows(path_manager, rows)
	}
	return errors.New("Filestore not initialized")
}

func (self *JournalService) Start(config_obj *config_proto.Config) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Journal service.")
	return nil
}

func StartJournalService(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) error {

	// It is valid to have a journal service with no configured datastore:
	// 1. Watchers will never be notified.
	// 2. PushRows() will fail with an error.
	service := &JournalService{}
	old_service, err := services.GetJournal()
	if err == nil {
		service.qm = old_service.(*JournalService).qm
	}

	qm, _ := file_store.GetQueueManager(config_obj)
	if qm != nil {
		service.qm = qm
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer services.RegisterJournal(nil)

		<-ctx.Done()
	}()

	services.RegisterJournal(service)

	return service.Start(config_obj)
}
