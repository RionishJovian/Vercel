package jellyfin

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Read OS counters and only mounted local volumes matching configured libraries.
// Never execute user commands, enumerate media files or stat arbitrary/UNC paths.
func collectHost(ctx context.Context, libraries []Library) *HostMetrics {
	h := &HostMetrics{OS: runtime.GOOS, Volumes: []Volume{}}
	if v, e := cpu.PercentWithContext(ctx, 250*time.Millisecond, false); e == nil && len(v) == 1 {
		h.CPU = &v[0]
	}
	if v, e := mem.VirtualMemoryWithContext(ctx); e == nil {
		h.Memory = &v.UsedPercent
		h.MemoryTotal = &v.Total
	}
	parts, e := localPartitions(ctx)
	if e != nil {
		return h
	}
	selected := map[string]bool{}
	for _, l := range libraries {
		for _, location := range l.Locations {
			best := ""
			for _, p := range parts {
				if !localFilesystem(p.Fstype) {
					continue
				}
				if withinMount(location, p.Mountpoint, runtime.GOOS == "windows") && len(p.Mountpoint) > len(best) {
					best = p.Mountpoint
				}
			}
			if best != "" {
				selected[best] = true
			}
		}
	}
	for _, p := range parts {
		if !selected[p.Mountpoint] {
			continue
		}
		delete(selected, p.Mountpoint)
		if len(h.Volumes) >= 20 {
			break
		}
		if v, e := disk.UsageWithContext(ctx, p.Mountpoint); e == nil && v.Total > 0 && v.Free <= v.Total {
			h.Volumes = append(h.Volumes, Volume{Label: p.Mountpoint, Total: v.Total, Free: v.Free})
		}
	}
	return h
}
func localFilesystem(fs string) bool {
	switch strings.ToLower(fs) {
	case "local-windows", "apfs", "hfs", "hfs+", "ext2", "ext3", "ext4", "xfs", "btrfs", "zfs", "ntfs", "refs", "fat", "fat32", "exfat", "vfat", "ufs":
		return true
	}
	return false
}
func withinMount(path, mount string, windows bool) bool {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") || mount == "" {
		return false
	}
	if windows {
		path = strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
		mount = strings.ToLower(strings.ReplaceAll(mount, `\`, "/"))
	} else {
		path = filepath.Clean(path)
		mount = filepath.Clean(mount)
	}
	mount = strings.TrimRight(mount, "/")
	return path == mount || strings.HasPrefix(path, mount+"/")
}
