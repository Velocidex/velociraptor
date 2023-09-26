//go:build linux
// +build linux

package linux

import (
	"context"
	"syscall"

	"github.com/Velocidex/ordereddict"

	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

type LinuxSysinfoFunction struct{}

func (self LinuxSysinfoFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "sysinfo",
		Doc:      "Collect system information on Linux clients",
		Metadata: vql.VQLMetadata().Permissions(acls.MACHINE_STATE).Build(),
	}
}

func (self LinuxSysinfoFunction) Call(ctx context.Context, scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.MACHINE_STATE)
	if err != nil {
		scope.Log("sysinfo: %s", err)
		return vfilter.Null{}
	}

	in := &syscall.Sysinfo_t{}
	err = syscall.Sysinfo(in)
	if err != nil {
		scope.Log("sysinfo: sysinfo() failed: %s", err)
		return vfilter.Null{}
	}

	return ordereddict.NewDict().
		Set("TotalPhysicalMemory", uint64(in.Totalram)*uint64(in.Unit)).
		Set("TotalFreeMemory", uint64(in.Freeram)*uint64(in.Unit)).
		Set("TotalSharedMemory", uint64(in.Sharedram)*uint64(in.Unit)).
		Set("TotalSwap", uint64(in.Totalswap)*uint64(in.Unit)).
		Set("FreeSwap", uint64(in.Freeswap)*uint64(in.Unit))
}

func init() {
	vql_subsystem.RegisterFunction(&LinuxSysinfoFunction{})
}
