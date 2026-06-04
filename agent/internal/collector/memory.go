package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type MemoryInfoResult struct {
	Total       uint64
	Free        uint64
	Available   uint64
	Used        uint64
	UsedPercent float64
	SwapTotal   uint64
	SwapFree    uint64
}

func MemoryInfo(procRoot string) (*MemoryInfoResult, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	if err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}
	return parseMeminfo(string(data))
}

func parseMeminfo(content string) (*MemoryInfoResult, error) {
	fields := map[string]uint64{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// 去掉 " kB" 后缀
		val = strings.TrimSuffix(val, " kB")
		val = strings.TrimSpace(val)
		if n, err := strconv.ParseUint(val, 10, 64); err == nil {
			fields[key] = n * 1024 // kB → bytes
		}
	}

	info := &MemoryInfoResult{
		Total:     fields["MemTotal"],
		Free:      fields["MemFree"],
		Available: fields["MemAvailable"],
		SwapTotal: fields["SwapTotal"],
		SwapFree:  fields["SwapFree"],
	}
	if info.Available == 0 {
		info.Available = info.Free
	}
	info.Used = info.Total - info.Available
	if info.Total > 0 {
		info.UsedPercent = float64(info.Used) / float64(info.Total) * 100
	}
	return info, nil
}
