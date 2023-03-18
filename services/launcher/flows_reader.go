package launcher

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
)

const (
	WORKERS = 100
)

type FlowReader struct {
	client_id string

	reader_ctx context.Context
	ctx        context.Context
	cancel     func()
	config_obj *config_proto.Config
	wg         *sync.WaitGroup

	// Feed client id here
	In chan<- string

	// Read records from here
	Out <-chan *flows_proto.ArtifactCollectorContext
	out chan *flows_proto.ArtifactCollectorContext
}

// Wait for all in flight requests to finish.
func (self *FlowReader) Close() {
	self.cancel()

	// Closing the input channel will cause all workers to quit
	close(self.In)

	// Wait for all workers to quit.
	self.wg.Wait()

	// Close the output channel to signal to listeners they are done.
	close(self.out)
}

// Cancel and abort in flight requests.
func (self *FlowReader) Cancel() {
	self.cancel()
}

func NewFlowReader(ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) *FlowReader {

	in := make(chan string)
	out := make(chan *flows_proto.ArtifactCollectorContext)
	wg := &sync.WaitGroup{}

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &FlowReader{
		client_id:  client_id,
		reader_ctx: ctx,
		ctx:        sub_ctx,
		cancel:     cancel,
		config_obj: config_obj,
		wg:         wg,
		In:         in,
		Out:        out,
		out:        out,
	}

	for i := 0; i < WORKERS; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for session_id := range in {
				collection_context, err := LoadCollectionContext(
					config_obj, client_id, session_id)
				if err == nil {
					select {
					case <-ctx.Done():
						return
					case out <- collection_context:
					}
				}
			}
		}()
	}

	return self
}
