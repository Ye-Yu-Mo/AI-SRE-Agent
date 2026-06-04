package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CPUInfoResult struct {
	Percent    float64
	NumCores   int
	ModelName  string
}

type cpuSample struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
}

func (s cpuSample) total() uint64 {
	return s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq + s.steal
}

func (s cpuSample) idleTotal() uint64 {
	return s.idle + s.iowait
}

func CPUInfo(procRoot string) (*CPUInfoResult, error) {
	s1, err := readCPUSample(procRoot)
	if err != nil {
		return nil, err
	}

	time.Sleep(100 * time.Millisecond)

	s2, err := readCPUSample(procRoot)
	if err != nil {
		return nil, err
	}

	totalDelta := s2.total() - s1.total()
	idleDelta := s2.idleTotal() - s1.idleTotal()

	var percent float64
	if totalDelta > 0 {
		percent = float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	}

	cores := countCPUCores(procRoot)
	model := readCPUModel(procRoot)

	return &CPUInfoResult{
		Percent:   percent,
		NumCores:  cores,
		ModelName: model,
	}, nil
}

func readCPUSample(procRoot string) (cpuSample, error) {
	data, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return cpuSample{}, fmt.Errorf("read stat: %w", err)
	}
	return parseCPUStat(string(data))
}

func parseCPUStat(content string) (cpuSample, error) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu  ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		// fields[0] = "cpu", fields[1..] = numbers
		nums := make([]uint64, 0, 8)
		for _, f := range fields[1:] {
			n, _ := strconv.ParseUint(f, 10, 64)
			nums = append(nums, n)
		}
		if len(nums) >= 8 {
			return cpuSample{
				user: nums[0], nice: nums[1], system: nums[2], idle: nums[3],
				iowait: nums[4], irq: nums[5], softirq: nums[6], steal: nums[7],
			}, nil
		}
	}
	return cpuSample{}, fmt.Errorf("no cpu line found in stat")
}

func countCPUCores(procRoot string) int {
	data, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return 1
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			count++
		}
	}
	if count == 0 {
		count = 1
	}
	return count
}

func readCPUModel(procRoot string) string {
	data, err := os.ReadFile(filepath.Join(procRoot, "cpuinfo"))
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}
