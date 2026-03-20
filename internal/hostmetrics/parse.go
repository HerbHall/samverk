package hostmetrics

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// ParseCgroupCPUStat parses the contents of /sys/fs/cgroup/cpu.stat (cgroup v2)
// and returns the cumulative CPU usage in microseconds.
// Exported so tests can feed canned data without touching the filesystem.
func ParseCgroupCPUStat(data []byte) (uint64, error) {
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("usage_usec ")) {
			fields := bytes.Fields(line)
			if len(fields) == 2 {
				return strconv.ParseUint(string(fields[1]), 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("hostmetrics: usage_usec not found in cpu.stat")
}

// ParseMemInfo parses the contents of /proc/meminfo and returns
// MemTotal, MemAvailable, SwapTotal, and SwapFree in bytes.
// Exported so tests can feed canned data without touching the filesystem.
func ParseMemInfo(data []byte) (total, available, swapTotal, swapFree uint64, err error) {
	var found int
	for _, line := range bytes.Split(data, []byte("\n")) {
		parts := strings.Fields(string(line))
		if len(parts) < 2 {
			continue
		}
		val, parseErr := strconv.ParseUint(parts[1], 10, 64)
		if parseErr != nil {
			continue
		}
		// Values in /proc/meminfo are in kB.
		valBytes := val * 1024
		switch parts[0] {
		case "MemTotal:":
			total = valBytes
			found++
		case "MemAvailable:":
			available = valBytes
			found++
		case "SwapTotal:":
			swapTotal = valBytes
			found++
		case "SwapFree:":
			swapFree = valBytes
			found++
		}
		if found >= 4 {
			break
		}
	}
	if found < 2 {
		return 0, 0, 0, 0, fmt.Errorf("incomplete meminfo: found %d of 4 fields", found)
	}
	return total, available, swapTotal, swapFree, nil
}

// ParseLoadAvg parses the contents of /proc/loadavg and returns the
// 1-minute, 5-minute, and 15-minute load averages.
// Exported so tests can feed canned data without touching the filesystem.
func ParseLoadAvg(data []byte) (load1, load5, load15 float64, err error) {
	fields := strings.Fields(string(bytes.TrimSpace(data)))
	if len(fields) < 3 {
		return 0, 0, 0, fmt.Errorf("loadavg: expected at least 3 fields, got %d", len(fields))
	}
	load1, err = strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loadavg field 0: %w", err)
	}
	load5, err = strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loadavg field 1: %w", err)
	}
	load15, err = strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("loadavg field 2: %w", err)
	}
	return load1, load5, load15, nil
}
