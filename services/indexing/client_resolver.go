package indexing

import (
	"context"
	"sync"

	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

const (
	WORKERS = 100
)

type ClientResolver struct {
	reader_ctx context.Context
	ctx        context.Context
	cancel     func()
	config_obj *config_proto.Config
	wg         *sync.WaitGroup

	// Feed client id here
	In chan<- string

	// Read records from here
	Out <-chan *api_proto.ApiClient
	out chan *api_proto.ApiClient
}

// Wait for all in flight requests to finish.
func (self *ClientResolver) Close() {
	self.cancel()

	// Closing the input channel will cause all workers to quit
	close(self.In)

	// Wait for all workers to quit.
	self.wg.Wait()

	// Close the output channel to signal to listeners they are done.
	close(self.out)
}

// Cancel and abort in flight requests.
func (self *ClientResolver) Cancel() {
	self.cancel()
}

func NewClientResolver(ctx context.Context,
	config_obj *config_proto.Config,
	indexer *Indexer) *ClientResolver {

	in := make(chan string)
	out := make(chan *api_proto.ApiClient)
	wg := &sync.WaitGroup{}

	sub_ctx, cancel := context.WithCancel(ctx)
	self := &ClientResolver{
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

			for client_id := range in {
				record, err := indexer.FastGetApiClient(
					self.ctx, self.config_obj, client_id)
				if err == nil {
					select {
					case <-ctx.Done():
						return
					case out <- record:
					}
				}
			}
		}()
	}

	return self
}
