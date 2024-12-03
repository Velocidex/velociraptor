package memcache

import (
	"context"
	"sync/atomic"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/vfilter"
)

func (self *MemcacheFileStore) WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	self.mu.Lock()

	stats := make([]*ordereddict.Dict, 0, len(self.data_cache))
	total_bytes := atomic.LoadInt64(&self.total_cached_bytes)

	for path, writer := range self.data_cache {
		row := ordereddict.NewDict().
			Set("Path", path).
			Set("TotalCachedBytes", total_bytes)
		row.MergeFrom(writer.Stats())

		stats = append(stats, row)
	}
	self.mu.Unlock()

	for _, row := range stats {
		output_chan <- row
	}
}
