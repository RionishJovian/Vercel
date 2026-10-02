//go:build !windows

package jellyfin

import (
	"context"
	"github.com/shirou/gopsutil/v4/disk"
)

func localPartitions(ctx context.Context) ([]disk.PartitionStat, error) {
	return disk.PartitionsWithContext(ctx, false)
}
