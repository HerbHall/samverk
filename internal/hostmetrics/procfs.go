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

// isLXCContainer reports whether the process is running inside an LXC container.
// In LXC containers /proc/loadavg reflects the host's load average (not namespaced),
// so callers should prefer cgroup-based CPU stats for accurate container metrics.
func isLXCContainer() bool {
	data, err := os.ReadFile("/proc/1/environ")
	if err != nil {
		return false
	}
	// /proc/1/environ is NUL-delimited; search for the container=lxc entry.
	for _, entry := range splitNUL(data) {
		if entry == "container=lxc" {
			return true
		}
	}
	return false
}

// splitNUL splits a NUL-delimited byte slice into strings.
func splitNUL(b []byte) []string {
	parts := make([]string, 0, 16)
	for _, part := range splitBytes(b, 0) {
		parts = append(parts, string(part))
	}
	return parts
}

// splitBytes splits b on the given delimiter byte.
func splitBytes(b []byte, delim byte) [][]byte {
	var out [][]byte
	for {
		i := indexByte(b, delim)
		if i < 0 {
			if len(b) > 0 {
				out = append(out, b)
			}
			break
		}
		out = append(out, b[:i])
		b = b[i+1:]
	}
	return out
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// readCgroupCPUStat reads /sys/fs/cgroup/cpu.stat (cgroup v2) and returns
// the cumulative CPU usage in microseconds.
func readCgroupCPUStat() (uint64, error) {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.stat")
	if err != nil {
		return 0, err
	}
	return ParseCgroupCPUStat(data)
}
