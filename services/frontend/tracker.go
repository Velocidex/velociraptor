package frontend

import (
	"context"

	"www.velocidex.com/golang/vfilter"
)

func (self *MasterFrontendManager) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	metrics := &FrontendMetrics{
		NodeName: "master",
	}

	err := calculateMetrics(metrics)
	if err != nil {
		return
	}
	output_chan <- metrics.ToDict().
		Set("HK Conc",
			self.housekeep_concurrency_gauge.Get())

	// Find all the currently connected minions

}

func (self *MinionFrontendManager) ProfileWriter(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	metrics := &FrontendMetrics{
		NodeName: self.name,
	}

	err := calculateMetrics(metrics)
	if err != nil {
		return
	}
	output_chan <- metrics.ToDict()
}
