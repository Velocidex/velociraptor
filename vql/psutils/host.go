package psutils

import (
	"context"
	"os"
	"runtime"

	"github.com/shirou/gopsutil/v4/host"
)

type InfoStat struct {
	host.InfoStat
}

// The gopsutil InfoWithContext() calls a number of dangerous
// functions which shell out causing performance issues. This is a
// reimplementation of that function with more careful calls.
func InfoWithContext(ctx context.Context) (*InfoStat, error) {
	ret := &InfoStat{host.InfoStat{
		OS: runtime.GOOS,
	}}
	ret.Hostname, _ = os.Hostname()
	ret.Platform, ret.PlatformFamily, ret.PlatformVersion, _ = PlatformInformationWithContext(ctx)
	ret.KernelVersion, _ = host.KernelVersionWithContext(ctx)
	ret.KernelArch, _ = host.KernelArch()
	ret.VirtualizationSystem, ret.VirtualizationRole, _ = host.VirtualizationWithContext(ctx)
	ret.BootTime, _ = host.BootTimeWithContext(ctx)
	ret.Uptime, _ = host.UptimeWithContext(ctx)
	ret.HostID = HostID()
	return ret, nil
}
