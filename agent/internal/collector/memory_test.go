package collector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryInfo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "meminfo", `MemTotal:       16384000 kB
MemFree:         4096000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          3072000 kB
SwapTotal:       8192000 kB
SwapFree:        7168000 kB
`)

	info, err := MemoryInfo(dir)
	if err != nil {
		t.Fatalf("MemoryInfo: %v", err)
	}

	if info.Total != 16384000*1024 {
		t.Errorf("Total = %d, want %d", info.Total, 16384000*1024)
	}
	if info.Free != 4096000*1024 {
		t.Errorf("Free = %d, want %d", info.Free, 4096000*1024)
	}
	if info.Available != 8192000*1024 {
		t.Errorf("Available = %d, want %d", info.Available, 8192000*1024)
	}
	if info.Used != info.Total-info.Available {
		t.Errorf("Used = %d, want %d", info.Used, info.Total-info.Available)
	}
	// UsedPercent
	expectedPct := float64(info.Used) / float64(info.Total) * 100
	if info.UsedPercent < expectedPct-0.1 || info.UsedPercent > expectedPct+0.1 {
		t.Errorf("UsedPercent = %.2f, want ~%.2f", info.UsedPercent, expectedPct)
	}
}

func TestMemoryInfo_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := MemoryInfo(dir)
	if err == nil {
		t.Error("expected error for missing meminfo")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}
