package throttler

import (
	"context"

	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

func ProfileWriter(ctx context.Context, scope vfilter.Scope,
	output_chan chan vfilter.Row) {
	stats.ProfileWriter(ctx, scope, output_chan)
}

func (self *statsCollector) ProfileWriter(
	ctx context.Context, scope vfilter.Scope, output_chan chan vfilter.Row) {
	self.mu.Lock()
	defer self.mu.Unlock()

	for _, k := range utils.Sort(self.throttlers) {
		t, _ := self.throttlers[k]
		output_chan <- t.Stats().
			Set("AvCPU", self.samples[1].average_cpu_load).
			Set("AvIOP", self.samples[1].average_iops).
			Set("Samples", self.sample_count)
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "Throttler",
		Description:   "Track operations of the Throttler",
		ProfileWriter: ProfileWriter,
		Categories:    []string{"Global", "Services"},
	})
}
