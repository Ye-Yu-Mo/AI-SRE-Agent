package deploy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type HealthStatus string

const (
	HealthPassing HealthStatus = "passing"
	HealthFailing HealthStatus = "failing"
	HealthUnknown HealthStatus = "unknown"
)

type HealthResult struct {
	Status     HealthStatus `json:"status"`
	LatencyMs  int64        `json:"latency_ms,omitempty"`
	StatusCode int          `json:"status_code,omitempty"`
	Port       int          `json:"port,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// HTTPHealthCheck 发 GET 请求检查 HTTP 服务
func HTTPHealthCheck(url string, expectedStatus int, timeout time.Duration) HealthResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	start := time.Now()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HealthResult{Status: HealthFailing, Error: err.Error()}
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()
	if expectedStatus > 0 && resp.StatusCode != expectedStatus {
		return HealthResult{
			Status:     HealthFailing,
			StatusCode: resp.StatusCode,
			LatencyMs:  latency,
			Error:      fmt.Sprintf("expected status %d, got %d", expectedStatus, resp.StatusCode),
		}
	}

	return HealthResult{Status: HealthPassing, StatusCode: resp.StatusCode, LatencyMs: latency}
}

// probeHealthOnPorts 依次探测候选端口，第一个通的返回 passing，全不通返回 failing。
func probeHealthOnPorts(ports []int, timeout time.Duration) HealthResult {
	for _, port := range ports {
		url := fmt.Sprintf("http://localhost:%d", port)
		r := HTTPHealthCheck(url, 0, timeout)
		r.Port = port
		if r.Status == HealthPassing {
			return r
		}
	}
	return HealthResult{Status: HealthFailing}
}

// ProbeAppHealth 对运行中应用做实时健康探测。
// workDir 指向应用的 compose 目录，用于读取 compose 文件获取端口映射；
// 若无法解析则回退到固定候选端口列表。
func ProbeAppHealth(appName, workDir string) HealthResult {
	ports := fixedPorts
	if workDir != "" {
		if custom := readComposePorts(workDir); len(custom) > 0 {
			ports = custom
		}
	}
	return probeHealthOnPorts(ports, 2*time.Second)
}

// TCPHealthCheck 拨号检查端口可达
func TCPHealthCheck(host string, port int, timeout time.Duration) HealthResult {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return HealthResult{Status: HealthFailing, Error: err.Error()}
	}
	conn.Close()

	return HealthResult{Status: HealthPassing, LatencyMs: time.Since(start).Milliseconds()}
}

// fixedPorts 默认探测端口列表。ProbeAppHealth 优先使用 compose 文件中的端口。
var fixedPorts = []int{80, 8080, 8888, 3000, 5000}

// readComposePorts 从 workDir 读取 compose 文件，提取 ports 段中映射到 host 的端口。
func readComposePorts(workDir string) []int {
	for _, name := range []string{"compose.yaml", "docker-compose.yml", "docker-compose.yaml"} {
		data, err := os.ReadFile(filepath.Join(workDir, name))
		if err != nil {
			continue
		}
		if ports := parseComposePorts(string(data)); len(ports) > 0 {
			return ports
		}
	}
	return nil
}

// parseComposePorts 从 YAML 内容中提取 host 端口。
// 支持格式: "8080:80", "3000", "0.0.0.0:8080:80"。
func parseComposePorts(content string) []int {
	var ports []int
	lines := strings.Split(content, "\n")
	inPorts := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "ports:") {
			inPorts = true
			continue
		}
		if inPorts {
			// 检测下一个顶级 key 或下一段缩进结束
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
				if !strings.HasPrefix(trimmed, "-") {
					inPorts = false
					continue
				}
			}
		}
		if !inPorts {
			continue
		}
		// 端口行: - "8080:80" 或 - 8080:80 或 - "80"
		val := strings.Trim(trimmed, "- \"")
		// 去掉方案前缀如 "8080:80/tcp"或引号
		val = strings.SplitN(val, "/", 2)[0]
		// 取第一个冒号前的端口（host 端口）："8080:80" → 8080, "80" → 80
		parts := strings.SplitN(val, ":", 2)
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if hostPort > 0 && hostPort < 65536 {
			ports = append(ports, hostPort)
		}
	}
	return ports
}
