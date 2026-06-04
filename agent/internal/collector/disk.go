package collector

import (
	"syscall"
)

type DiskInfoResult struct {
	Total       uint64
	Free        uint64
	Used        uint64
	UsedPercent float64
}

func DiskInfo(path string) (*DiskInfoResult, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	var usedPercent float64
	if total > 0 {
		usedPercent = float64(used) / float64(total) * 100
	}

	return &DiskInfoResult{
		Total:       total,
		Free:        free,
		Used:        used,
		UsedPercent: usedPercent,
	}, nil
}
