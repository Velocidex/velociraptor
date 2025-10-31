package faults

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	FaultInjector = &FaultInjectorService{}
)

// The fault injector is used to inject faults into various parts of
// the system during debug and test phases.
type FaultInjectorService struct {
	blockHTTPDo     time.Duration
	timeStep        time.Duration
	mockTimeRestore func()
}

func (self *FaultInjectorService) SetBlockHTTPDo(d time.Duration) {
	self.blockHTTPDo = d
}

// Block a HTTPDo function to emulate TCP time wait blocakge.
func (self *FaultInjectorService) BlockHTTPDo(ctx context.Context) {
	if self.blockHTTPDo == 0 {
		return
	}
	utils.SleepWithCtx(ctx, self.blockHTTPDo)
}

// Force all times to step forward by the specified amount.
func (self *FaultInjectorService) SetTimeStep(d time.Duration) {
	if self.mockTimeRestore != nil {
		self.mockTimeRestore()
		self.mockTimeRestore = nil
	}
	if d > 0 {
		self.mockTimeRestore = utils.MockTime(
			utils.RealClockWithOffset{Duration: d})
	}
}
