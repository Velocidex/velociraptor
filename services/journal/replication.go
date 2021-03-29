package journal

// A replicating journal service replicates all events to the master
// and receives events from the master node.

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/Velocidex/ordereddict"
	"github.com/pkg/errors"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/directory"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
)

type ReplicationService struct {
	config_obj *config_proto.Config
	api_client api_proto.APIClient
	closer     func() error // Closer for the api_client

	file_buffer *directory.FileBasedRingBuffer
	tmpfile     *os.File
}

func (self *ReplicationService) Start(
	ctx context.Context, wg *sync.WaitGroup) (err error) {

	// Are we the master?
	api_client, closer, err := services.Frontend.GetMasterAPIClient(ctx)
	if err != nil {
		return errors.Wrap(err, "ReplicationService: ")
	}

	self.tmpfile, err = ioutil.TempFile("", "replication")
	if err != nil {
		return err
	}

	self.file_buffer, err = directory.NewFileBasedRingBuffer(
		self.config_obj, self.tmpfile)
	if err != nil {
		return err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		defer self.Close()

		<-ctx.Done()
	}()

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>Starting</> Replication service to node %v.",
		services.Frontend.GetMasterName())

	self.api_client = api_client
	self.closer = closer

	return nil
}

func (self *ReplicationService) PushRowsToArtifact(
	config_obj *config_proto.Config,
	rows []*ordereddict.Dict, artifact, client_id, flow_id string) error {

	// FIXME: implement buffer file here.
	ctx := context.Background()

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
	logger.Info("<green>ReplicationService</> Sending %v rows to %v.", len(rows), artifact)

	_, err = self.api_client.PushEvents(ctx, request)
	return err
}

func (self *ReplicationService) Watch(ctx context.Context, queue string) (
	<-chan *ordereddict.Dict, func()) {

	output_chan := make(chan *ordereddict.Dict)

	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Info("<green>ReplicationService</>: Watching for events from %v", queue)

	// Failed to connect with the master. What should we do here?
	subctx, cancel := context.WithCancel(ctx)
	stream, err := self.api_client.WatchEvent(subctx, &api_proto.EventRequest{
		Queue: queue,
	})
	if err != nil {
		close(output_chan)
		return output_chan, func() {}
	}

	go func() {
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				break
			}

			if err != nil {
				continue
			}
			logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
			dict := ordereddict.NewDict()
			err = dict.UnmarshalJSON(event.Jsonl)
			if err == nil {
				select {
				case <-ctx.Done():
					return

				case output_chan <- dict:
					logger.Debug("ReplicationService: Received event on %v: %v\n", queue, dict)
				}
			}

		}
	}()

	return output_chan, cancel
}

func (self *ReplicationService) Close() {
	if self.closer != nil {
		self.closer()
	}
	self.file_buffer.Close()
	os.Remove(self.tmpfile.Name()) // clean up file buffer
}
