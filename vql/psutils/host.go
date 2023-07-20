package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v3/host"
)

type InfoStat struct {
	host.InfoStat
}

func InfoWithContext(ctx context.Context) (*InfoStat, error) {
	res, err := host.InfoWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return &InfoStat{InfoStat: *res}, err
}
