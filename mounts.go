package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

type Mount struct {
	Device string `json:"device"`
	Path   string `json:"path"`
	FSType string `json:"fstype"`
	Total  int64  `json:"total"`
	Free   int64  `json:"free"`
}

// realFSTypes lists filesystems that represent actual storage (not
// pseudo/virtual filesystems like proc, sysfs, tmpfs, cgroup...).
var realFSTypes = map[string]bool{
	"ext4": true, "ext3": true, "ext2": true,
	"xfs": true, "btrfs": true, "zfs": true,
	"ntfs": true, "ntfs3": true, "vfat": true, "exfat": true, "fat32": true,
	"f2fs": true, "reiserfs": true, "jfs": true, "hfsplus": true,
	"fuseblk": true,
	"nfs":     true, "nfs4": true, "cifs": true,
}

func parseMountsFile(path string) ([]Mount, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// seen tracks the shortest-path mount per device to deduplicate bind
	// mounts (e.g. container overlay storage re-mounting the same device).
	seen := make(map[string]int)
	var mounts []Mount
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		device, mountPath, fstype := fields[0], unescapeMountPath(fields[1]), fields[2]
		if !realFSTypes[fstype] {
			continue
		}
		m := Mount{Device: device, Path: mountPath, FSType: fstype}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPath, &stat); err == nil {
			m.Total = int64(stat.Blocks) * int64(stat.Bsize)
			m.Free = int64(stat.Bavail) * int64(stat.Bsize)
		}
		if idx, exists := seen[device]; exists {
			if len(mountPath) < len(mounts[idx].Path) {
				mounts[idx] = m
			}
			continue
		}
		seen[device] = len(mounts)
		mounts = append(mounts, m)
	}
	return mounts, sc.Err()
}

// unescapeMountPath decodes the octal escapes /proc/mounts uses for
// spaces, tabs, newlines and backslashes (e.g. "\040" → " ").
func unescapeMountPath(p string) string {
	if !strings.Contains(p, `\`) {
		return p
	}
	r := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return r.Replace(p)
}

func getMounts() ([]Mount, error) {
	return parseMountsFile("/proc/mounts")
}
