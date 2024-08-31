//go:build windows
// +build windows

package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v3/host"
)

func PlatformInformationWithContext(ctx context.Context) (platform string, family string, version string, err error) {
	return host.PlatformInformationWithContext(ctx)
}
