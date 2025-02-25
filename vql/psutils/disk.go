package psutils

import (
	"context"

	"github.com/shirou/gopsutil/v4/disk"
)

type UsageStat struct {
	disk.UsageStat
}

type PartitionStat struct {
	disk.PartitionStat
}

func Usage(mount string) (*UsageStat, error) {
	usage, err := disk.Usage(mount)
	if err != nil {
		return nil, err
	}

	return &UsageStat{*usage}, nil
}

func SerialNumber(disk_name string) (string, error) {
	return disk.SerialNumber(disk_name)
}

func PartitionsWithContext(ctx context.Context) ([]PartitionStat, error) {
	res, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	result := make([]PartitionStat, 0, len(res))
	for _, i := range res {
		result = append(result, PartitionStat{i})
	}

	return result, nil
}
