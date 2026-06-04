package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

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
	srv := newServer(cfg, plan.NewStore(), auditStore, ln)

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

func TestServerSecretRejection(t *testing.T) {
	dir := t.TempDir()
	ln := listen(t)

	cfg := &Config{
		Dir:    dir,
		Secret: "correct-secret",
	}
	auditStore, _ := storage.NewStore(t.TempDir())
	srv := newServer(cfg, plan.NewStore(), auditStore, ln)

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
