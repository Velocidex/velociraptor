package throttler

import (
	"context"

	"github.com/Velocidex/ordereddict"
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
	var rows []*ordereddict.Dict
	self.mu.Lock()
	for _, k := range utils.Sort(self.throttlers) {
		t, _ := self.throttlers[k]
		rows = append(rows, t.Stats().
			Set("AvCPUPercent", int(self.samples[1].average_cpu_load)).
			Set("AvIOP", int(self.samples[1].average_iops)).
			Set("Samples", self.sample_count))
	}
	self.mu.Unlock()

	for _, row := range rows {
		output_chan <- row
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
