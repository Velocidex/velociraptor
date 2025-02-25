//go:build darwin && !cgo
// +build darwin,!cgo

package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v4/host"
)

func cmdNameWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func cmdlineSliceWithContext(ctx context.Context, pid int32) ([]string, error) {
	return nil, NotImplementedError
}

func TimesWithContext(ctx context.Context, pid int32) (*TimesStat, error) {
	return nil, NotImplementedError
}

func ExeWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func CwdWithContext(ctx context.Context, pid int32) (string, error) {
	return "", NotImplementedError
}

func MemoryInfoWithContext(ctx context.Context, pid int32) (*MemoryInfoStat, error) {
	return nil, NotImplementedError
}

// This is really slow and shells out but it is ok for the nocgo debug
// build. The CGO production build calls the right API.
func HostID() string {
	id, _ := host.HostID()
	return id
}
