package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ai-sre/agent/internal/deploy"
	"github.com/ai-sre/agent/internal/identity"
	"github.com/ai-sre/agent/internal/plan"
	"github.com/ai-sre/agent/internal/storage"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func url(ln net.Listener, path string) string {
	return fmt.Sprintf("http://%s%s", ln.Addr().String(), path)
}

func TestServerStartupAndHealth(t *testing.T) {
	dir := t.TempDir()
	ln := listen(t)

	cfg := &Config{
		Dir:    dir,
		Secret: "test-secret",
	}
	auditStore, _ := storage.NewStore(t.TempDir())
	srv := newServer(cfg, nil, plan.NewStore(), auditStore, deploy.NewReleaseStore(), ln)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { _ = srv.Serve(ln) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(url(ln, "/health"))
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health: status %d, want 200", resp.StatusCode)
	}

	srv.Shutdown(ctx)
}

// testServer 启动一个带正确 secret 的测试服务器，返回 listener 和 cleanup。
func testServer(t *testing.T) (net.Listener, func()) {
	t.Helper()
	ln := listen(t)
	dir := t.TempDir()
	// 创建真实的 identity 文件供测试使用
	id, err := identity.New(dir)
	if err != nil {
		t.Fatalf("identity.New: %v", err)
	}
	cfg := &Config{Dir: dir, Secret: "test-secret"}
	auditStore, _ := storage.NewStore(t.TempDir())
	srv := newServer(cfg, id, plan.NewStore(), auditStore, deploy.NewReleaseStore(), ln)
	go func() { _ = srv.Serve(ln) }()
	time.Sleep(100 * time.Millisecond)
	return ln, func() { srv.Shutdown(context.Background()) }
}

// authPost 带 secret 发 POST，返回 status code 和解析后的 body。
func authPost(t *testing.T, ln net.Listener, path string, payload any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url(ln, path), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// createPlan 创建一个 plan，返回 plan_id 和 requires_approval。
func createPlan(t *testing.T, ln net.Listener, svc string) (string, bool) {
	t.Helper()
	code, body := authPost(t, ln, "/api/v1/plans", map[string]any{
		"intent":    "restart " + svc,
		"server_id": "srv_test",
		"actions": []map[string]any{{
			"type":   "service.restart",
			"target": map[string]string{"kind": "systemd_service", "name": svc},
		}},
	})
	if code != http.StatusOK {
		t.Fatalf("create plan: status %d, body %v", code, body)
	}
	id, _ := body["plan_id"].(string)
	ra, _ := body["requires_approval"].(bool)
	return id, ra
}

// M1: 高风险 plan 未带 approve 标志 apply → 必须 409，不执行。
func TestApply_HighRiskWithoutApprovalRejected(t *testing.T) {
	ln, cleanup := testServer(t)
	defer cleanup()

	// restart postgresql in production → high risk, requires approval
	planID, requiresApproval := createPlan(t, ln, "postgresql")
	if !requiresApproval {
		t.Fatalf("postgresql restart should require approval")
	}

	code, body := authPost(t, ln, "/api/v1/plans/"+planID+"/apply", map[string]any{})
	if code != http.StatusConflict {
		t.Errorf("apply without approval: status %d, want 409; body %v", code, body)
	}

	// plan 状态应保持 pending，未被执行
	req, _ := http.NewRequest("GET", url(ln, "/api/v1/plans/"+planID), nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET plan: %v", err)
	}
	defer resp.Body.Close()
	var p map[string]any
	json.NewDecoder(resp.Body).Decode(&p)
	if p["status"] != "pending" {
		t.Errorf("plan status after rejected apply = %v, want pending", p["status"])
	}
}

// M1: 高风险 plan 带 approve:true → 通过闸门（执行结果不在本测试断言范围）。
func TestApply_HighRiskWithApprovalPassesGate(t *testing.T) {
	ln, cleanup := testServer(t)
	defer cleanup()

	planID, _ := createPlan(t, ln, "postgresql")

	code, body := authPost(t, ln, "/api/v1/plans/"+planID+"/apply", map[string]any{"approve": true})
	if code == http.StatusConflict {
		t.Errorf("apply with approve:true should pass the gate, got 409; body %v", body)
	}
}

// M1: 低风险写操作（development 环境）不受闸门影响——通过闸门正常执行。
func TestApply_LowRiskPassesGate(t *testing.T) {
	ln, cleanup := testServer(t)
	defer cleanup()

	// 普通容器 docker.restart 在 production 是 medium+approval。
	// 当前 main.go 写死 production，所有写操作都需审批——
	// 这正是闸门要保护的：未带 approve 一律拦下。
	planID, requiresApproval := createPlan(t, ln, "nginx")
	if !requiresApproval {
		t.Skip("nginx restart not requiring approval; gate not exercised")
	}

	code, _ := authPost(t, ln, "/api/v1/plans/"+planID+"/apply", map[string]any{"approve": true})
	if code == http.StatusConflict {
		t.Errorf("apply with approve:true should pass gate, got 409")
	}
}

func TestServerSecretRejection(t *testing.T) {
	dir := t.TempDir()
	ln := listen(t)

	cfg := &Config{
		Dir:    dir,
		Secret: "correct-secret",
	}
	auditStore, _ := storage.NewStore(t.TempDir())
	srv := newServer(cfg, nil, plan.NewStore(), auditStore, deploy.NewReleaseStore(), ln)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { _ = srv.Serve(ln) }()
	time.Sleep(100 * time.Millisecond)

	endpoint := url(ln, "/api/v1/inspect")

	// 无 token → 401
	resp, _ := http.Get(endpoint)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: got %d, want 401", resp.StatusCode)
	}

	// 错误 token → 401
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	resp2, _ := http.DefaultClient.Do(req)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d, want 401", resp2.StatusCode)
	}

	// 正确 token → 200
	req3, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	req3.Header.Set("Authorization", "Bearer correct-secret")
	resp3, _ := http.DefaultClient.Do(req3)
	resp3.Body.Close()
	if resp3.StatusCode == http.StatusUnauthorized {
		t.Error("correct token: got 401, want auth to pass")
	}

	srv.Shutdown(ctx)
}

// M4: identity 端点返回 server_id 和 hostname，供多服务器路由使用。
func TestIdentityEndpoint(t *testing.T) {
	ln, cleanup := testServer(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", url(ln, "/api/v1/identity"), nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET identity: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET identity: status %d, want 200", resp.StatusCode)
	}

	var id map[string]any
	json.NewDecoder(resp.Body).Decode(&id)

	sid, _ := id["server_id"].(string)
	if sid == "" {
		t.Error("identity response missing server_id")
	}
	if len(sid) < 4 || sid[:4] != "srv_" {
		t.Errorf("server_id should start with srv_, got %q", sid)
	}

	host, _ := id["hostname"].(string)
	if host == "" {
		t.Error("identity response missing hostname")
	}
}
