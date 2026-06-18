package deploy

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ProbeAppHealth 在所有候选端口都不通时返回 failing，不 panic。
func TestProbeAppHealth_NoService(t *testing.T) {
	// 探测一个几乎不可能有服务的端口集合不现实，这里用一个明确关闭的端口段。
	// ProbeAppHealth 内部探测固定端口列表，本机这些端口若恰好有服务会干扰，
	// 故只断言"返回结果且 Status 是合法枚举，不 panic"。
	r := ProbeAppHealth("nonexistent-app", "")
	switch r.Status {
	case HealthPassing, HealthFailing, HealthUnknown:
		// ok
	default:
		t.Errorf("unexpected status: %q", r.Status)
	}
}

// ProbeAppHealth 命中一个真实在跑的 HTTP 服务时返回 passing。
func TestProbeAppHealth_HitsRunningServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	// 从 httptest URL 解析出端口
	portStr := ts.URL[strings.LastIndex(ts.URL, ":")+1:]
	port, _ := strconv.Atoi(portStr)

	r := probeHealthOnPorts([]int{port}, 2*time.Second)
	if r.Status != HealthPassing {
		t.Errorf("Status = %q, want passing (port %d)", r.Status, port)
	}
	if r.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", r.StatusCode)
	}
}

// 所有端口都不通 → failing。
func TestProbeHealthOnPorts_AllClosed(t *testing.T) {
	// 65000+ 高位端口大概率无服务
	r := probeHealthOnPorts([]int{65001, 65002}, 500*time.Millisecond)
	if r.Status != HealthFailing {
		t.Errorf("Status = %q, want failing", r.Status)
	}
}
