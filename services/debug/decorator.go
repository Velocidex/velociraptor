package debug

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

func Decorate(
	ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row,
	writer func(ctx context.Context, scope vfilter.Scope, in_chan chan vfilter.Row),
	transform func(item *ordereddict.Dict) *ordereddict.Dict) {

	in_chan := make(chan vfilter.Row)
	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		for row := range in_chan {
			item := vfilter.RowToDict(ctx, scope, row)
			select {
			case <-ctx.Done():
				return

			case output_chan <- transform(item):
			}
		}
	}()

	writer(ctx, scope, in_chan)
	close(in_chan)

	wg.Wait()
}
