//go:build !linux

package hostmetrics

func readMemInfo() (total, available, swapTotal, swapFree uint64, err error) {
	return 0, 0, 0, 0, nil
}

func readLoadAvg() (load1, load5, load15 float64, err error) {
	return 0, 0, 0, nil
}

func readDiskUsage(_ string) (total, used uint64, err error) {
	return 0, 0, nil
}
