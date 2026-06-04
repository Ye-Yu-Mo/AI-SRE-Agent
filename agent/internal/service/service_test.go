package service

import (
	"strings"
	"testing"
)

func TestUnitTemplate(t *testing.T) {
	cfg := UnitConfig{
		BinaryPath:  "/usr/local/bin/ai-server-agent",
		DataDir:     "/var/lib/ai-server-agent",
		User:        "ai-server-agent",
		Group:       "ai-server-agent",
		Port:        9090,
		Secret:      "test-secret",
	}

	unit := RenderUnit(cfg)

	// 必须有的关键指令
	assertContains(t, unit, "[Unit]")
	assertContains(t, unit, "Description=AI Server Agent")
	assertContains(t, unit, "After=network.target")

	assertContains(t, unit, "[Service]")
	assertContains(t, unit, "Type=simple")
	assertContains(t, unit, "User=ai-server-agent")
	assertContains(t, unit, "Restart=always")
	assertContains(t, unit, "RestartSec=5")
	assertContains(t, unit, "ExecStart=/usr/local/bin/ai-server-agent")
	assertContains(t, unit, "--dir /var/lib/ai-server-agent")
	assertContains(t, unit, "--port 9090")

	assertContains(t, unit, "[Install]")
	assertContains(t, unit, "WantedBy=multi-user.target")

	// Secret 不应出现在 unit 文件中（安全）
	if strings.Contains(unit, cfg.Secret) {
		t.Error("unit file must not contain the secret in plaintext")
	}
}

func TestUnitTemplate_EnvironmentFile(t *testing.T) {
	cfg := UnitConfig{
		BinaryPath:  "/usr/local/bin/ai-server-agent",
		DataDir:     "/var/lib/ai-server-agent",
		User:        "ai-server-agent",
		Group:       "ai-server-agent",
		Port:        9090,
		Secret:      "test-secret",
		EnvFile:     "/etc/ai-server-agent/env",
	}

	unit := RenderUnit(cfg)
	assertContains(t, unit, "EnvironmentFile=/etc/ai-server-agent/env")
	// Secret 不在 unit 中
	assertNotContains(t, unit, "test-secret")
}

func TestUnitTemplate_DefaultValues(t *testing.T) {
	cfg := UnitConfig{
		BinaryPath: "/usr/local/bin/ai-server-agent",
		User:       "ai-server-agent",
		Group:      "ai-server-agent",
	}

	unit := RenderUnit(cfg)
	// DataDir 默认值
	assertContains(t, unit, "--dir /var/lib/ai-server-agent")
	// Port 默认值
	assertContains(t, unit, "--port 9090")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected string to contain %q", substr)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected string to NOT contain %q", substr)
	}
}
