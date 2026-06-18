package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ConfigureCaddy 在 Caddy 未安装时返回错误而不是 panic
func TestConfigureCaddy_NoBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	err := ConfigureCaddy("app.example.com", "3000")
	if err == nil {
		t.Error("expected error when caddy binary not found")
	}
	if !strings.Contains(err.Error(), "caddy") {
		t.Errorf("error should mention caddy: %v", err)
	}
}

// ConfigureCaddy 生成正确的 Caddyfile block
func TestConfigureCaddy_GeneratesBlock(t *testing.T) {
	got := generateCaddyBlock("app.example.com", "3000")
	if !strings.Contains(got, "app.example.com") {
		t.Errorf("block missing domain: %s", got)
	}
	if !strings.Contains(got, "reverse_proxy localhost:3000") {
		t.Errorf("block missing reverse_proxy: %s", got)
	}
}

// RemoveCaddyRoute 删除匹配 domain 的 block
func TestRemoveCaddyRoute_RemovesBlock(t *testing.T) {
	dir := t.TempDir()
	caddyfile := filepath.Join(dir, "Caddyfile")
	content := "example.com {\n\treverse_proxy localhost:8080\n}\n\nother.com {\n\treverse_proxy localhost:3000\n}\n"
	os.WriteFile(caddyfile, []byte(content), 0644)

	err := removeCaddyRouteFile(caddyfile, "example.com")
	if err != nil {
		t.Fatalf("removeCaddyRouteFile: %v", err)
	}

	remain, _ := os.ReadFile(caddyfile)
	if strings.Contains(string(remain), "example.com") {
		t.Error("example.com block was not removed")
	}
	if !strings.Contains(string(remain), "other.com") {
		t.Error("other.com block was incorrectly removed")
	}
}

// RemoveCaddyRoute 文件不存在时不报错
func TestRemoveCaddyRoute_MissingFile(t *testing.T) {
	err := removeCaddyRouteFile("/nonexistent/caddyfile", "example.com")
	if err != nil {
		t.Errorf("missing file should not error: %v", err)
	}
}
