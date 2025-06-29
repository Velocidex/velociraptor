package journal

// A replicating journal service replicates all events to the master
// and receives events from the master node.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/tempfile"
	utils_tempfile "www.velocidex.com/golang/velociraptor/utils/tempfile"
)

var (
	replicationSendHistorgram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "replication_minion_latency",
			Help:    "Latency to send replication messages from minion to the master.",
			Buckets: prometheus.LinearBuckets(0.1, 1, 10),
		},
	)

	replicationItemSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "replication_item_size",
			Help: "Size of replicated message.",
			Buckets: prometheus.LinearBuckets(
				100*1024, 1024*1024, 10),
		},
	)

	replicationTotalSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "replication_service_total_send",
		Help: "Total number of PushRow rpc calls.",
	})

	replicationTotalReceive = promauto.NewCounter(prometheus.CounterOpts{
		Name: "replication_service_total_receive",
		Help: "Total number of Events received from the master rpc calls.",
	})

	replicationTotalSendErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "replication_service_total_send_errors",
		Help: "Total number of PushRow rpc calls.",
	})
)

type jsonBatch struct {
	bytes.Buffer
	row_count int
}

type ReplicationService struct {
	config_obj *config_proto.Config
	Buffer     *BufferFile
	tmpfile    *os.File
	ctx        context.Context

	// Locally connected watchers.
	qm api.QueueManager

	sender chan *api_proto.PushEventRequest

	// Synchronizes access to files. NOTE: This only works within
	// process!
	mu            sync.Mutex
	locks         map[string]*sync.Mutex
	retryDuration time.Duration

	// The set of events the master is interested in.
	masterRegistrations map[string]bool

	// Store rows for async push
	batch map[string]*jsonBatch
}

func (self *ReplicationService) RetryDuration() time.Duration {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.retryDuration
}

func (self *ReplicationService) SetRetryDuration(duration time.Duration) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.retryDuration = duration
}

func (self *ReplicationService) isEventRegistered(artifact string) bool {
	self.mu.Lock()
	defer self.mu.Unlock()

	ok, pres := self.masterRegistrations[artifact]
	return pres && ok
}

func (self *ReplicationService) pumpEventFromBufferFile() error {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	frontend_manager, err := services.GetFrontendManager(self.config_obj)
	if err != nil {
		return err
	}

	for {
		event, err := self.Buffer.Lease()
		// No events available or some other error, sleep and
		// try again later.
		if err != nil {
			select {
			case <-self.ctx.Done():
				return nil

			case <-time.After(utils.Jitter(self.RetryDuration())):
				continue
			}
		}

		// Retry to send the event.
		for {
			// Get a new API handle each time in case it became invalid.
			api_client, closer, err := frontend_manager.GetMasterAPIClient(
				self.ctx)
			if err != nil {
				logger.Error("<red>ReplicationService %v</>Unable to connect %v",
					services.GetOrgName(self.config_obj), err)
				time.Sleep(time.Second)
				continue
			}

			_, err = api_client.PushEvents(self.ctx, event)
			if err == nil {
				err := closer()
				if err != nil {
					return err
				}
				break
			}

			// We are unable to send it, sleep and
			// try again later.
			select {
			case <-self.ctx.Done():
				err := closer()
				if err != nil {
					return err
				}
				return nil

			case <-time.After(utils.Jitter(self.RetryDuration())):
				err := closer()
				if err != nil {
					logger.Error("<red>ReplicationService</> %v", err)
				}
			}

		}
	}
}

// Periodically flush the batches built up during
// PushRowsToArtifactAsync calls.
func (self *ReplicationService) startAsyncLoop(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) {

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(utils.Jitter(200 * time.Millisecond)):
				// Work on the batch without a lock
				self.mu.Lock()
				todo := self.batch
				self.batch = make(map[string]*jsonBatch)
				self.mu.Unlock()

				for k, v := range todo {
					// Ignore errors since there is no way to report
					// to the caller.
					_ = self.PushJsonlToArtifact(ctx,
						config_obj, v.Bytes(), v.row_count, k,
						"server", "")
				}
			}
		}
	}()
}

func (self *ReplicationService) Start(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) (err error) {

	// If we are the master node we do not replicate anywhere.
	frontend_manager, err := services.GetFrontendManager(config_obj)
	if err != nil {
		return err
	}

	// Initialize our default values and start the service for
	// real.
	self.ctx = ctx

	// Do not have channel buffer because then we might lose events on
	// restart. Events will flow on to the buffer file when the gRPC
	// client is too busy.
	self.sender = make(chan *api_proto.PushEventRequest)
	self.SetRetryDuration(5 * time.Second)

	self.tmpfile, err = tempfile.TempFile("replication")
	if err != nil {
		return err
	}
	utils_tempfile.AddTmpFile(self.tmpfile.Name())

	self.Buffer, err = NewBufferFile(self.config_obj, self.tmpfile)
	if err != nil {
		return err
	}

	go func() {
		err := self.pumpEventFromBufferFile()
		if err != nil {
			logger := logging.GetLogger(
				self.config_obj, &logging.FrontendComponent)
			logger.Error("<red>pumpEventFromBufferFile</>: %v", err)
		}
	}()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer self.Close()

			for {
				select {
				case <-ctx.Done():
					return

					// Read events from the channel and
					// try to send them
				case request, ok := <-self.sender:
					if !ok {
						return
					}

					timer := prometheus.NewTimer(
						prometheus.ObserverFunc(func(v float64) {
							replicationSendHistorgram.Observe(v)
						}))

					api_client, closer, err := frontend_manager.GetMasterAPIClient(ctx)
					if err != nil {
						continue
					}

					_, err = api_client.PushEvents(ctx, request)
					timer.ObserveDuration()

					if err != nil {
						replicationTotalSendErrors.Inc()

						// Attempt to push the events
						// to the buffer file instead
						// for later delivery.
						_ = self.Buffer.Enqueue(request)
					}
					_ = closer()
				}
			}
		}()
	}

	// Startup the async writer. This is used to queue up multiple
	// small events to write in larger chunks for gRPC efficiency.
	self.startAsyncLoop(ctx, wg, config_obj)

	self.startMasterRegistrationLoop(ctx, wg, config_obj)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("<green>Starting</> Replication service %v to master frontend at %v",
		services.GetOrgName(config_obj),
		grpc_client.GetAPIConnectionString(self.config_obj))

	return nil
}

func (self *ReplicationService) ProcessMasterRegistrations(event *ordereddict.Dict) {
	names_any, pres := event.Get("Events")
	if !pres {
		return
	}

	names, ok := names_any.([]interface{})
	if ok {
		// -----
		self.mu.Lock()
		self.masterRegistrations = make(map[string]bool)

		for _, name := range names {
			name_str, ok := name.(string)
			if ok {
				self.masterRegistrations[name_str] = true
			}
		}
		logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
		logger.WithFields(logrus.Fields{
			"events": names,
		}).Info("Master event registrations")
		self.mu.Unlock()
		// -----
	}
}

func (self *ReplicationService) startMasterRegistrationLoop(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) {

	events, cancel := self.Watch(ctx,
		"Server.Internal.MasterRegistrations", "ReplicationService")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-events:
				if !ok {
					return
				}
				self.ProcessMasterRegistrations(event)
			}
		}
	}()

}

func (self *ReplicationService) AppendJsonlToResultSet(
	config_obj *config_proto.Config,
	path api.FSPathSpec,
	jsonl []byte, row_count int) error {

	// Key a lock to manage access to this file.
	self.mu.Lock()
	key := path.AsClientPath()
	mu, pres := self.locks[key]
	if !pres {
		mu = &sync.Mutex{}
		self.locks[key] = mu
	}
	self.mu.Unlock()

	// Lock the file.
	mu.Lock()
	defer mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path, json.DefaultEncOpts(), utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return err
	}

	rs_writer.WriteJSONL(jsonl, uint64(row_count))
	rs_writer.Close()

	return nil
}

func (self *ReplicationService) AppendToResultSet(
	config_obj *config_proto.Config,
	path api.FSPathSpec,
	rows []*ordereddict.Dict,
	options services.JournalOptions) error {

	// Key a lock to manage access to this file.
	self.mu.Lock()
	key := path.AsClientPath()
	mu, pres := self.locks[key]
	if !pres {
		mu = &sync.Mutex{}
		self.locks[key] = mu
	}
	self.mu.Unlock()

	// Lock the file.
	mu.Lock()
	defer mu.Unlock()

	file_store_factory := file_store.GetFileStore(config_obj)

	sync := utils.BackgroundWriter
	if options.Sync {
		sync = utils.SyncCompleter
	}

	rs_writer, err := result_sets.NewResultSetWriter(file_store_factory,
		path, json.DefaultEncOpts(),
		sync, result_sets.AppendMode)
	if err != nil {
		return err
	}

	for _, row := range rows {
		rs_writer.Write(row)
	}

	rs_writer.Close()

	return nil
}

func (self *ReplicationService) Broadcast(
	ctx context.Context, config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flows_id string) error {

	return notInitializedError
}

func (self *ReplicationService) PushRowsToArtifactAsync(
	ctx context.Context, config_obj *config_proto.Config,
	row *ordereddict.Dict, artifact string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	queue, pres := self.batch[artifact]
	if !pres {
		queue = &jsonBatch{}
	}

	serialized, err := row.MarshalJSON()
	if err == nil {
		queue.Write(serialized)
		queue.Write([]byte("\n"))
	}
	self.batch[artifact] = queue
}

func (self *ReplicationService) pushRowsToLocalQueueManager(
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

func (self *ReplicationService) pushJsonlToLocalQueueManager(
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

func (self *ReplicationService) PushJsonlToArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	jsonl []byte, row_count int,
	artifact, client_id, flow_id string) error {

	err := self.pushJsonlToLocalQueueManager(ctx,
		config_obj, jsonl, row_count, artifact,
		client_id, flow_id)
	if err != nil {
		return err
	}

	// Do not replicate the event if the master does not care about it.
	if !self.isEventRegistered(artifact) {
		return nil
	}

	replicationTotalSent.Inc()

	replicationItemSize.Observe(float64(len(jsonl)))
	request := &api_proto.PushEventRequest{
		Artifact: artifact,
		ClientId: client_id,
		FlowId:   flow_id,
		Jsonl:    jsonl,
		OrgId:    self.config_obj.OrgId,
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("<green>ReplicationService %v</> Sending %v bytes to %v for %v.",
		services.GetOrgName(config_obj),
		len(jsonl), artifact, client_id)

	// Should not block! If the channel is full we save the event
	// into the file buffer for later.
	select {
	case self.sender <- request:
		return nil
	default:
		return self.Buffer.Enqueue(request)
	}
}

func (self *ReplicationService) PushRowsToArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flow_id string) error {

	serialized, err := json.MarshalJsonl(rows)
	if err != nil {
		return err
	}
	replicationItemSize.Observe(float64(len(serialized)))

	err = self.pushRowsToLocalQueueManager(ctx,
		config_obj, rows, artifact, client_id, flow_id)
	if err != nil {
		return err
	}

	// Do not replicate the event if the master does not care about it.
	if !self.isEventRegistered(artifact) {
		return nil
	}

	replicationTotalSent.Inc()

	request := &api_proto.PushEventRequest{
		Artifact: artifact,
		ClientId: client_id,
		FlowId:   flow_id,
		Jsonl:    serialized,
		Rows:     int64(len(rows)),
		OrgId:    self.config_obj.OrgId,
	}

	// Should not block! If the channel is full we save the event
	// into the file buffer for later.
	select {
	case self.sender <- request:
		return nil

	default:
		return self.Buffer.Enqueue(request)
	}
}

func (self *ReplicationService) GetWatchers() []string {
	return nil
}

// Watch the master for new events
func (self *ReplicationService) Watch(
	ctx context.Context, queue, watcher_name string) (
	<-chan *ordereddict.Dict, func()) {

	output_chan := make(chan *ordereddict.Dict)
	subctx, cancel := context.WithCancel(ctx)

	go func() {
		for {
			in_chan := self.watchOnce(subctx, queue, watcher_name)

		retry:
			for {
				select {
				case event, ok := <-in_chan:
					if !ok {
						break retry
					}
					output_chan <- event
				}
			}

			// Keep retrying to reconnect in case the
			// connection dropped.
			last_try := utils.GetTime().Now()

			select {
			case <-self.ctx.Done():
				return

			case <-time.After(utils.Jitter(self.RetryDuration())):

				// Avoid retrying too quickly. This is mainly for
				// tests where the time is mocked for the After(delay)
				// above does not work.
				if utils.GetTime().Now().Sub(last_try) < time.Minute {
					utils.SleepWithCtx(ctx, time.Minute)
				}
			}

			logger := logging.GetLogger(self.config_obj,
				&logging.FrontendComponent)
			logger.Info("<green>ReplicationService Reconnect %s</> %s: "+
				"Watch for events from %v",
				services.GetOrgName(self.config_obj), watcher_name, queue)
		}
	}()

	return output_chan, cancel
}

// Try to connect to the API handler once and return in case of
// failure.
func (self *ReplicationService) watchOnce(
	ctx context.Context,
	queue, watcher_name string) <-chan *ordereddict.Dict {

	output_chan := make(chan *ordereddict.Dict)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>ReplicationService %s</> %s: Watching for events from %v",
		services.GetOrgName(self.config_obj), watcher_name, queue)

	subctx, cancel := context.WithCancel(ctx)

	frontend_manager, err := services.GetFrontendManager(self.config_obj)
	if err != nil {
		logger.Error("<red>ReplicationService %v</> Unable to connect %v",
			services.GetOrgName(self.config_obj), err)
		close(output_chan)
		cancel()
		return output_chan
	}

	api_client, closer, err := frontend_manager.GetMasterAPIClient(ctx)
	if err != nil {
		logger.Error("<red>ReplicationService %v</> Unable to connect %v",
			services.GetOrgName(self.config_obj), err)
		close(output_chan)
		cancel()
		return output_chan
	}

	stream, err := api_client.WatchEvent(subctx, &api_proto.EventRequest{
		OrgId: self.config_obj.OrgId,
		Queue: queue,
		WatcherName: watcher_name + "_" +
			services.GetNodeName(self.config_obj.Frontend),
	})
	if err != nil {
		close(output_chan)
		_ = closer()
		cancel()
		return output_chan
	}

	go func() {
		defer func() {
			cancel()
			close(output_chan)
			_ = closer()
		}()

		for {
			event, err := stream.Recv()
			if err != nil {
				return
			}

			replicationTotalReceive.Inc()

			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			dict := ordereddict.NewDict()
			err = dict.UnmarshalJSON(event.Jsonl)
			if err == nil {
				select {
				case <-ctx.Done():
					return

				case output_chan <- dict:
					logger.Debug("<green>ReplicationService %v</>: Received event on %v\n",
						services.GetOrgName(self.config_obj), queue)
				}
			}
		}
	}()

	return output_chan
}

func (self *ReplicationService) Close() {
	self.Buffer.Close()
	err := os.Remove(self.tmpfile.Name()) // clean up file buffer
	utils_tempfile.RemoveTmpFile(self.tmpfile.Name(), err)
}

func NewReplicationService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (
	*ReplicationService, error) {

	service := &ReplicationService{
		config_obj:          config_obj,
		locks:               make(map[string]*sync.Mutex),
		masterRegistrations: make(map[string]bool),
		batch:               make(map[string]*jsonBatch),
	}

	qm, err := file_store.GetQueueManager(config_obj)
	if err != nil || qm != nil {
		service.qm = qm
	}

	err = service.Start(ctx, config_obj, wg)
	return service, err
}
