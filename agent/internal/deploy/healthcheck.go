package deploy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
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

// ProbeAppHealth 对运行中应用做实时健康探测，探测固定候选端口集合。
// workDir 暂未使用，预留给后续从 compose 文件解析端口。
func ProbeAppHealth(_ string, _ string) HealthResult {
	return probeHealthOnPorts([]int{80, 8080, 8888, 3000, 5000}, 2*time.Second)
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
