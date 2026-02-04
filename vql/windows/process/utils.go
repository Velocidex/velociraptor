//go:build windows
// +build windows

package process

import (
	"context"
	"fmt"

	"www.velocidex.com/golang/velociraptor/vql/tools/process"
	"www.velocidex.com/golang/vfilter"
)

func GetProcessContext(
	ctx context.Context, scope vfilter.Scope, pid uint64) string {
	tracker := process.GetGlobalTracker()
	record, ok := tracker.Get(ctx, scope, fmt.Sprintf("%v", pid))
	if ok {
		name, _ := record.Data().GetString("Name")
		if name == "" {
			name = "unknown"
		}
		return fmt.Sprintf(" %v (%v) ", record.Id, name)
	}

	return fmt.Sprintf(" %v ", pid)
}
