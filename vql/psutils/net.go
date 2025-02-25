package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v4/net"
)

type ConnectionStat struct {
	net.ConnectionStat
}

func ConnectionsWithContext(
	ctx context.Context, kind string) ([]ConnectionStat, error) {
	res, err := net.ConnectionsWithContext(ctx, kind)
	if err != nil {
		return nil, err
	}

	result := make([]ConnectionStat, 0, len(res))
	for _, i := range res {
		result = append(result, ConnectionStat{ConnectionStat: i})
	}
	return result, nil
}
