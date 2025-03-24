//go:build !linux && !windows

package throttler

import "context"

// A Dummy CPU Reporter used on platforms where we do not do
// throttling.
type CPUReporter struct{}

func (self *CPUReporter) GetCpuTime(ctx context.Context) float64 {
	return 0
}

func (self *CPUReporter) Close() {}

func NewCPUReporter() *CPUReporter {
	return &CPUReporter{}
}

type IOPSReporter struct{}

func NewIOPSReporter() *IOPSReporter {
	return &IOPSReporter{}
}

func (self *IOPSReporter) GetIops(ctx context.Context) float64 {
	return 0
}

func (self *IOPSReporter) Close() {}
