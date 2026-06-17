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
