package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v3/cpu"
)

func CountsWithContext(ctx context.Context, logical bool) (int, error) {
	return cpu.CountsWithContext(ctx, logical)
}
