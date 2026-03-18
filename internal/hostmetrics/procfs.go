//go:build linux

package hostmetrics

import (
	"os"
	"syscall"
)

func readMemInfo() (total, available, swapTotal, swapFree uint64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return ParseMemInfo(data)
}

func readLoadAvg() (load1, load5, load15 float64, err error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, err
	}
	return ParseLoadAvg(data)
}

func readDiskUsage(path string) (total, used uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bsize := uint64(stat.Bsize) //nolint:gosec // G115: Bsize is always positive on Linux
	total = stat.Blocks * bsize
	free := stat.Bfree * bsize
	used = total - free
	return total, used, nil
}
