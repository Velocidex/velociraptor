//go:build linux
// +build linux

package ebpf

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/services/debug"
	"www.velocidex.com/golang/vfilter"
)

func WriteProfile(ctx context.Context,
	scope vfilter.Scope, output_chan chan vfilter.Row) {

	if gEbpfManager == nil {
		output_chan <- ordereddict.NewDict().
			Set("Error", "EBPF Manager not initialized yet - run the watch_ebpf() plugin to initialize.")

	} else {
		stats := gEbpfManager.Stats()
		idle := ""
		if stats.NumberOfListeners == 0 {
			idle = stats.IdleTime.String()
		}

		output_chan <- ordereddict.NewDict().
			Set("EBFProgramStatus", stats.EBFProgramStatus).
			Set("NumberOfListeners", stats.NumberOfListeners).
			Set("EIDMonitored", stats.EIDMonitored).
			Set("IdleTime", idle).
			Set("IdleUnloadTimeout", stats.IdleUnloadTimeout.String())
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "ebpf_manager",
		Description:   "Current state of the ebpf manager",
		ProfileWriter: WriteProfile,
	})

}
