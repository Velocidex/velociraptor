package broadcast

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

func (self *BroadcastService) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()
	defer self.mu.Unlock()

	queue_stats := self.pool.Stats()
	for _, k := range queue_stats.Keys() {
		v, _ := queue_stats.Get(k)
		stats, ok := v.([]*ordereddict.Dict)
		if ok {
			for _, s := range stats {
				output_chan <- s
			}
		}
	}
}
