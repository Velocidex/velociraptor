// The journal service receives events from various sources and writes
// them to storage. Velociraptor uses the artifact name and source as
// the name of the queue that will be written.

// The service will also allow for registration of interested events
// and will deliver events to interested parties.

// We use the underlying file store's queue manager to actually manage
// the notifications and watching and write the events to storage.
package journal

import (
	"context"
	"errors"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/alitto/pond/v2"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	notInitializedError = errors.New("Not initialized")

	PushRowsToArtifactAsyncIsSynchrnous = false
)

type JournalService struct {
	config_obj *config_proto.Config
	qm         api.QueueManager

	// Synchronizes access to files. NOTE: This only works within
	// process!
	mu    sync.Mutex
	locks map[string]*sync.Mutex

	pool pond.Pool
}

func (self *JournalService) GetWatchers() []string {
	return self.qm.GetWatchers()
}

func (self *JournalService) publishWatchers(ctx context.Context) {
	_ = self.PushRowsToInternalEventArtifact(ctx, self.config_obj,
		[]*ordereddict.Dict{ordereddict.NewDict().
			Set("Events", self.GetWatchers())},
		"Server.Internal.MasterRegistrations")
}

func (self *JournalService) Watch(
	ctx context.Context, queue_name string,
	watcher_name string) (<-chan *ordereddict.Dict, func()) {

	if self == nil || self.qm == nil {
		// Readers block on nil channel.
		return nil, func() {}
	}

	disable_file_buffering := false
	if self.config_obj.Frontend != nil &&
		self.config_obj.Frontend.Resources != nil {
		disable_file_buffering = self.config_obj.Frontend.Resources.DisableFileBuffering
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("%s: Watching for events from %v", watcher_name, queue_name)
	res, cancel := self.qm.Watch(ctx, queue_name, &api.QueueOptions{
		OwnerName:            watcher_name,
		DisableFileBuffering: disable_file_buffering,
	})

	// Advertise new watchers
	self.publishWatchers(ctx)

	return res, func() {
		cancel()

		// Advertise that a watcher was removed.
		self.publishWatchers(ctx)
	}
}

// Write rows to a simple result set. This function manages concurrent
// access to the result set within the same frontend. Currently there
// is no need to manage write concurrency across frontends because
// clients can only talk with a single frontend at the time.
func (self *JournalService) AppendToResultSet(
	config_obj *config_proto.Config,
	path api.FSPathSpec,
	rows []*ordereddict.Dict,
	options services.JournalOptions) error {

	// Key a lock to manage access to this file.
	self.mu.Lock()
	key := path.AsClientPath()
	per_file_mu, pres := self.locks[key]
	if !pres {
		per_file_mu = &sync.Mutex{}
		self.locks[key] = per_file_mu
	}
	self.mu.Unlock()

	// Lock the file.
	per_file_mu.Lock()
	defer per_file_mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)

	sync := utils.BackgroundWriter
	if options.Sync {
		sync = utils.SyncCompleter
	}

	// Append the data to the end of the file.
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path, json.DefaultEncOpts(), sync, result_sets.AppendMode)
	if err != nil {
		return err
	}

	for _, row := range rows {
		rs_writer.Write(row)
	}

	rs_writer.Close()

	return nil
}

func (self *JournalService) AppendJsonlToResultSet(
	config_obj *config_proto.Config,
	path api.FSPathSpec,
	jsonl []byte, row_count int) error {

	// Key a lock to manage access to this file.
	self.mu.Lock()
	key := path.AsClientPath()
	per_file_mu, pres := self.locks[key]
	if !pres {
		per_file_mu = &sync.Mutex{}
		self.locks[key] = per_file_mu
	}
	self.mu.Unlock()

	// Lock the file.
	per_file_mu.Lock()
	defer per_file_mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)

	// Append the data to the end of the file.
	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path, json.DefaultEncOpts(), utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return err
	}
	rs_writer.WriteJSONL(jsonl, uint64(row_count))
	rs_writer.Close()

	return nil
}

func (self *JournalService) PushRowsToArtifactAsync(
	ctx context.Context, config_obj *config_proto.Config, row *ordereddict.Dict,
	artifact string) {

	f := func() {
		err := self.PushRowsToArtifact(ctx, config_obj, []*ordereddict.Dict{row},
			artifact, "server", "")
		if err != nil {
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			logger.Error("<red>PushRowsToArtifactAsync</> %v", err)
		}
	}

	// If the pool is full, run this synchronously. This is necessary
	// to prevent deadlocks because some of the tasks may also call
	// the journal service to write asynchronously which they can not
	// do when taking up a pool slot.
	if self.pool.WaitingTasks() > 0 || PushRowsToArtifactAsyncIsSynchrnous {
		f()
		return
	}

	self.pool.Submit(f)
}

func (self *JournalService) Broadcast(
	ctx context.Context, config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flows_id string) error {
	if self == nil || self.qm == nil {
		return notInitializedError
	}

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, client_id, flows_id, artifact)
	if err != nil {
		return err
	}

	self.qm.Broadcast(path_manager, rows)
	return nil
}

func (self *JournalService) PushJsonlToArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	jsonl []byte, row_count int, artifact, client_id, flows_id string) error {

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, client_id, flows_id, artifact)
	if err != nil {
		return err
	}

	// Just a regular artifact, append to the existing result set.
	if !path_manager.IsEvent() {
		path, err := path_manager.GetPathForWriting()
		if err != nil {
			return err
		}
		return self.AppendJsonlToResultSet(config_obj, path, jsonl, row_count)
	}

	// The Queue manager will manage writing event artifacts to a
	// timed result set, including multi frontend synchronisation.
	if self != nil && self.qm != nil {
		return self.qm.PushEventJsonl(path_manager, jsonl, row_count)
	}
	return errors.New("Filestore not initialized")
}

func (self *JournalService) PushRowsToArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flows_id string) error {

	path_manager, err := artifacts.NewArtifactPathManager(ctx,
		config_obj, client_id, flows_id, artifact)
	if err != nil {
		return err
	}

	// Just a regular artifact, append to the existing result set.
	if !path_manager.IsEvent() {
		path, err := path_manager.GetPathForWriting()
		if err != nil {
			return err
		}
		return self.AppendToResultSet(config_obj, path, rows,
			services.JournalOptions{})
	}

	// The Queue manager will manage writing event artifacts to a
	// timed result set, including multi frontend synchronisation.
	if self != nil && self.qm != nil {
		return self.qm.PushEventRows(path_manager, rows)
	}
	return errors.New("Filestore not initialized")
}

func (self *JournalService) PushRowsToInternalEventArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact string) error {

	path_manager := artifacts.NewArtifactPathManagerWithMode(
		config_obj, "server", "F.Monitoring", artifact, paths.INTERNAL)
	if self != nil && self.qm != nil {
		return self.qm.PushEventRows(path_manager, rows)
	}
	return nil
}

func (self *JournalService) Start(config_obj *config_proto.Config) error {
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Journal service for %v.",
		services.GetOrgName(config_obj))

	return nil
}

func NewJournalService(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) (services.JournalService, error) {
	// Are we running on a minion frontend? If so we try to start
	// our replication service.
	if !services.IsMaster(config_obj) {
		j, err := NewReplicationService(ctx, wg, config_obj)
		return j, err
	}

	// It is valid to have a journal service with no configured datastore:
	// 1. Watchers will never be notified.
	// 2. PushRowsToArtifact() will fail with an error.
	service := &JournalService{
		config_obj: config_obj,
		locks:      make(map[string]*sync.Mutex),
		pool:       pond.NewPool(100),
	}

	qm, err := file_store.GetQueueManager(config_obj)
	if err == nil && qm != nil {
		service.qm = qm
	}

	return service, service.Start(config_obj)
}
