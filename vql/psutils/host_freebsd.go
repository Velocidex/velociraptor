//go:build freebsd
// +build freebsd

package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v4/host"
)

func PlatformInformationWithContext(ctx context.Context) (string, string, string, error) {
	return host.PlatformInformationWithContext(ctx)
}
