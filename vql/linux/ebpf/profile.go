//go:build linux && (arm64 || amd64)
// +build linux
// +build arm64 amd64

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

		if len(stats.Listeners) == 0 {
			output_chan <- ordereddict.NewDict().
				Set("EBFProgramStatus", stats.EBFProgramStatus).
				Set("IdleTime", stats.IdleTime.String()).
				Set("IdleUnloadTimeout", stats.IdleUnloadTimeout.String())
			return
		}

		for _, l := range stats.Listeners {
			output_chan <- ordereddict.NewDict().
				Set("EBFProgramStatus", stats.EBFProgramStatus).
				Set("IdleTime", "").
				Set("IdleUnloadTimeout", stats.IdleUnloadTimeout.String()).
				Set("PoilcyID", l.PolicyID).
				Set("Poilcy", l.Policy).
				Set("EventCount", l.EventCount).
				Set("EIDMonitored", l.EIDMonitored)
		}
	}
}

func init() {
	debug.RegisterProfileWriter(debug.ProfileWriterInfo{
		Name:          "ebpf_manager",
		Description:   "Current state of the ebpf manager",
		ProfileWriter: WriteProfile,
		Categories:    []string{"Global", "VQL", "Plugins"},
	})

}
