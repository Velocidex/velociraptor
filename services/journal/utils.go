package journal

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
)

// Watch a queue and apply a processor on any rows received.
func WatchQueueWithCB(ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup,
	artifact string,

	// A processor for rows from the queue
	processor func(ctx context.Context,
		config_obj *config_proto.Config,
		row *ordereddict.Dict) error) error {

	journal, err := services.GetJournal()
	if err != nil {
		return err
	}
	qm_chan, cancel := journal.Watch(ctx, artifact)

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()

		for {
			select {
			case row, ok := <-qm_chan:
				if !ok {
					return
				}
				processor(ctx, config_obj, row)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
