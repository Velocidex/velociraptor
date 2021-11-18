package journal

// A replicating journal service replicates all events to the master
// and receives events from the master node.

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/grpc_client"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/services"
)

var (
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

type ReplicationService struct {
	config_obj *config_proto.Config
	Buffer     *BufferFile
	tmpfile    *os.File
	ctx        context.Context

	sender chan *api_proto.PushEventRequest

	api_client api_proto.APIClient
	closer     func() error

	// Synchronizes access to files. NOTE: This only works within
	// process!
	mu            sync.Mutex
	locks         map[string]*sync.Mutex
	retryDuration time.Duration

	// Store rows for async push
	batch map[string][]*ordereddict.Dict
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

func (self *ReplicationService) pumpEventFromBufferFile() {
	for {
		event, err := self.Buffer.Lease()
		// No events available or some other error, sleep and
		// try again later.
		if err != nil {
			select {
			case <-self.ctx.Done():
				return

			case <-time.After(self.RetryDuration()):
				continue
			}
		}

		// Retry to send the event.
		for {
			_, err := self.api_client.PushEvents(self.ctx, event)
			if err == nil {
				break
			}
			// We are unable to send it, sleep and
			// try again later.
			select {
			case <-self.ctx.Done():
				return

			case <-time.After(self.RetryDuration()):
				continue
			}
		}
	}
}

func (self *ReplicationService) startAsyncLoop(
	ctx context.Context, wg *sync.WaitGroup, config_obj *config_proto.Config) {

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return

			case <-time.After(time.Second):
				// Take a copy to work on without a lock.
				todo := make(map[string][]*ordereddict.Dict)
				self.mu.Lock()
				fmt.Printf("Checking async cache: %v\n", len(self.batch))

				for k, v := range self.batch {
					if len(v) > 0 {
						todo[k] = v
					}
					delete(self.batch, k)
				}
				self.mu.Unlock()

				for k, v := range todo {
					// Ignore errors since there is no way to report
					// to the caller.
					fmt.Printf("Pushing %v rows to %v\n", len(v), k)
					_ = self.PushRowsToArtifact(config_obj, v, k, "server", "")
				}
			}
		}
	}()
}

func (self *ReplicationService) Start(
	ctx context.Context,
	config_obj *config_proto.Config, wg *sync.WaitGroup) (err error) {

	// If we are the master node we do not replicate anywhere.
	api_client, closer, err := services.GetFrontendManager().
		GetMasterAPIClient(ctx)
	if err != nil {
		return err
	}

	// Initialize our default values and start the service for
	// real.
	self.api_client = api_client
	self.closer = closer
	self.ctx = ctx
	self.sender = make(chan *api_proto.PushEventRequest, 100)
	self.SetRetryDuration(time.Second)

	self.tmpfile, err = ioutil.TempFile("", "replication")
	if err != nil {
		return err
	}

	self.Buffer, err = NewBufferFile(self.config_obj, self.tmpfile)
	if err != nil {
		return err
	}

	go self.pumpEventFromBufferFile()

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
					_, err := self.api_client.PushEvents(ctx, request)
					if err != nil {
						replicationTotalSendErrors.Inc()

						// Attempt to push the events
						// to the buffer file instead
						// for later delivery.
						_ = self.Buffer.Enqueue(request)
					}
				}
			}
		}()
	}

	// Startup the async writer. This is used to queue up multiple
	// small events to write in larger chunks for gRPC efficiency.
	self.startAsyncLoop(ctx, wg, config_obj)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("<green>Starting</> Replication service to master frontend at %v",
		grpc_client.GetAPIConnectionString(self.config_obj))

	return nil
}

func (self *ReplicationService) AppendToResultSet(
	config_obj *config_proto.Config,
	path api.FSPathSpec,
	rows []*ordereddict.Dict) error {

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
		path, nil, false /* truncate */)
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
	config_obj *config_proto.Config, rows []*ordereddict.Dict,
	artifact, client_id, flows_id string) error {

	return notInitializedError
}

func (self *ReplicationService) PushRowsToArtifactAsync(
	config_obj *config_proto.Config, row *ordereddict.Dict,
	artifact string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	queue, pres := self.batch[artifact]
	if !pres {
		queue = make([]*ordereddict.Dict, 0)
	}

	queue = append(queue, row)
	self.batch[artifact] = queue
}

func (self *ReplicationService) PushRowsToArtifact(
	config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flow_id string) error {

	replicationTotalSent.Inc()

	serialized, err := json.MarshalJsonl(rows)
	if err != nil {
		return err
	}

	request := &api_proto.PushEventRequest{
		Artifact: artifact,
		ClientId: client_id,
		FlowId:   flow_id,
		Jsonl:    serialized,
	}

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Debug("<green>ReplicationService</> Sending %v rows to %v for %v.",
		len(rows), artifact, client_id)

	// Should not block! If the channel is full we save the event
	// into the file buffer for later.
	select {
	case self.sender <- request:
		return nil
	default:
		return self.Buffer.Enqueue(request)
	}
}

func (self *ReplicationService) Watch(ctx context.Context, queue string) (
	<-chan *ordereddict.Dict, func()) {

	output_chan := make(chan *ordereddict.Dict)
	subctx, cancel := context.WithCancel(ctx)

	go func() {
		for {
			// Keep retrying to reconnect in case the
			// connection dropped.
			for event := range self.watchOnce(subctx, queue) {
				output_chan <- event
			}

			select {
			case <-self.ctx.Done():
				return
			case <-time.After(self.RetryDuration()):
			}

			logger := logging.GetLogger(self.config_obj,
				&logging.FrontendComponent)
			logger.Info("<green>ReplicationService Reconnect</>: "+
				"Watch for events from %v", queue)
		}
	}()

	return output_chan, cancel
}

// Try to connect to the API handler once and return in case of
// failure.
func (self *ReplicationService) watchOnce(ctx context.Context, queue string) <-chan *ordereddict.Dict {

	output_chan := make(chan *ordereddict.Dict)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>ReplicationService</>: Watching for events from %v", queue)

	subctx, cancel := context.WithCancel(ctx)

	stream, err := self.api_client.WatchEvent(subctx, &api_proto.EventRequest{
		Queue: queue,
	})
	if err != nil {
		close(output_chan)
		return output_chan
	}

	go func() {
		defer close(output_chan)
		defer cancel()

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
					logger.Debug("<green>ReplicationService</>: Received event on %v: %v\n", queue, dict)
				}
			}

		}
	}()

	return output_chan
}

func (self *ReplicationService) Close() {
	self.closer()
	self.Buffer.Close()
	os.Remove(self.tmpfile.Name()) // clean up file buffer
}
