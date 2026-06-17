package collector

import (
	"strings"
	"testing"
)

func TestParseDockerPS(t *testing.T) {
	// 模拟 docker ps --format json 输出
	output := `{"Names":"web-1","Image":"myapp:latest","Status":"Up 2 hours ago","Ports":"0.0.0.0:8888->80/tcp","CreatedAt":"2026-06-17 08:00:00 +0000 UTC"}
{"Names":"db-1","Image":"postgres:14","Status":"Up 5 days ago","Ports":"5432/tcp","CreatedAt":"2026-06-12 08:00:00 +0000 UTC"}
{"Names":"redis-1","Image":"redis:7","Status":"Up 5 days ago","Ports":"","CreatedAt":"2026-06-12 08:00:00 +0000 UTC"}
`

	containers := parseDockerPS(strings.NewReader(output))
	if len(containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(containers))
	}

	// 第一个容器
	c1 := containers[0]
	if c1.Name != "web-1" {
		t.Errorf("Name = %q, want web-1", c1.Name)
	}
	if c1.Image != "myapp:latest" {
		t.Errorf("Image = %q, want myapp:latest", c1.Image)
	}
	if c1.Status != "Up 2 hours ago" {
		t.Errorf("Status = %q, want Up 2 hours ago", c1.Status)
	}
	if len(c1.Ports) != 1 || c1.Ports[0] != "0.0.0.0:8888->80/tcp" {
		t.Errorf("Ports = %v", c1.Ports)
	}

	// 第二个容器
	c2 := containers[1]
	if c2.Name != "db-1" {
		t.Errorf("Name = %q, want db-1", c2.Name)
	}

	// 第三个容器 — 无端口映射
	c3 := containers[2]
	if len(c3.Ports) != 0 && c3.Ports[0] != "" {
		t.Errorf("Ports should be empty, got %v", c3.Ports)
	}
}

func TestParseDockerPS_Empty(t *testing.T) {
	containers := parseDockerPS(strings.NewReader(""))
	if len(containers) != 0 {
		t.Errorf("expected 0 containers, got %d", len(containers))
	}
}

func TestParseDockerPS_InvalidJSON(t *testing.T) {
	// 部分无效行不应导致整体失败
	output := `{"Names":"web-1","Image":"myapp","Status":"Up","Ports":"","CreatedAt":""}
garbage line
{"Names":"db-1","Image":"postgres","Status":"Up","Ports":"","CreatedAt":""}
`
	containers := parseDockerPS(strings.NewReader(output))
	if len(containers) != 2 {
		t.Errorf("expected 2 valid containers, got %d", len(containers))
	}
}
