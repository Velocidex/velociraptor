package event_logs

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/evtx"
	vfilter "www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type workerJob struct {
	wg          *sync.WaitGroup
	scope       vfilter.Scope
	output_chan chan types.Row
	chunk       *evtx.Chunk
	resolver    evtx.MessageResolver
}

func (self *workerJob) Run(ctx context.Context) {
	defer self.wg.Done()

	records, _ := self.chunk.Parse(0)
	for _, i := range records {
		self.scope.ChargeOp()

		event_map, ok := i.Event.(*ordereddict.Dict)
		if !ok {
			continue
		}
		event, pres := ordereddict.GetMap(event_map, "Event")
		if !pres {
			continue
		}

		if self.resolver != nil {
			event.Set("Message", evtx.ExpandMessage(event, self.resolver))
		}

		select {
		case <-ctx.Done():
			return

		case self.output_chan <- event:
		}
	}
}

type pool struct {
	wg          sync.WaitGroup
	in_chan     chan *workerJob
	output_chan chan types.Row
	ctx         context.Context
	cancel      func()
}

func (self *pool) Close() {
	close(self.in_chan)

	self.wg.Wait()

	self.cancel()
}

func (self *pool) Run(
	scope vfilter.Scope,
	chunk *evtx.Chunk,
	resolver evtx.MessageResolver) {
	job := &workerJob{
		wg:          &self.wg,
		scope:       scope,
		output_chan: self.output_chan,
		chunk:       chunk,
		resolver:    resolver,
	}

	// Wait for all chunks to be read.
	self.wg.Add(1)
	select {
	case <-self.ctx.Done():
		return
	case self.in_chan <- job:
	}
}

func newPool(
	ctx context.Context,
	output_chan chan types.Row, size int,
	resolver evtx.MessageResolver) *pool {

	subctx, cancel := context.WithCancel(ctx)

	result := &pool{
		in_chan:     make(chan *workerJob),
		ctx:         subctx,
		cancel:      cancel,
		output_chan: output_chan,
	}

	// Wait for all workers to quit
	result.wg.Add(size)
	for i := 0; i < size; i++ {
		go func() {
			defer result.wg.Done()

			for {
				select {
				case <-subctx.Done():
					return

				case job, ok := <-result.in_chan:
					if !ok {
						return
					}
					job.Run(subctx)
				}
			}
		}()
	}

	return result
}
