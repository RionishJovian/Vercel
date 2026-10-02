package jellyfin

import (
	"context"
	"github.com/shirou/gopsutil/v4/disk"
	"golang.org/x/sys/windows"
)

// Filter drive types before volume queries: gopsutil's Windows partition
// enumerator also queries remote drives, which this connector must not do.
func localPartitions(ctx context.Context) ([]disk.PartitionStat, error) {
	mask, e := windows.GetLogicalDrives()
	if e != nil {
		return nil, e
	}
	parts := []disk.PartitionStat{}
	for i := 0; i < 26; i++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if mask&(1<<i) == 0 {
			continue
		}
		path := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(path)
		kind := windows.GetDriveType(p)
		if kind == windows.DRIVE_FIXED || kind == windows.DRIVE_REMOVABLE {
			parts = append(parts, disk.PartitionStat{Mountpoint: path, Fstype: "local-windows"})
		}
	}
	return parts, nil
}
