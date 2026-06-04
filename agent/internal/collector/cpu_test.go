package collector

import (
	"testing"
)

func TestParseCPUStat(t *testing.T) {
	content := `cpu  1123456 12345 456789 98765432 12345 0 6789 0 0 0
cpu0 561728 6172 228394 49382716 6172 0 3394 0 0 0
cpu1 561728 6173 228395 49382716 6173 0 3395 0 0 0
`
	sample, err := parseCPUStat(content)
	if err != nil {
		t.Fatalf("parseCPUStat: %v", err)
	}
	if sample.user != 1123456 {
		t.Errorf("user = %d, want 1123456", sample.user)
	}
	if sample.idle != 98765432 {
		t.Errorf("idle = %d, want 98765432", sample.idle)
	}
}

func TestParseCPUStat_NoCPULine(t *testing.T) {
	content := `intr 12345
ctxt 67890
`
	_, err := parseCPUStat(content)
	if err == nil {
		t.Error("expected error for missing cpu line")
	}
}

func TestCountCPUCores(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "stat", `cpu  1 2 3 4 5 6 7 8 0 0
cpu0 1 2 3 4 5 6 7 8 0 0
cpu1 1 2 3 4 5 6 7 8 0 0
cpu2 1 2 3 4 5 6 7 8 0 0
cpu3 1 2 3 4 5 6 7 8 0 0
`)
	if n := countCPUCores(dir); n != 4 {
		t.Errorf("cores = %d, want 4", n)
	}
}

func TestCountCPUCores_Empty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "stat", "")
	if n := countCPUCores(dir); n != 1 {
		t.Errorf("cores = %d, want 1 (fallback)", n)
	}
}
