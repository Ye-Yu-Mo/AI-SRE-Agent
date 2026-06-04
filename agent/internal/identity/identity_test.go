package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewIdentity(t *testing.T) {
	dir := t.TempDir()
	id, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// 所有字段都得有值
	if id.ServerID == "" {
		t.Error("ServerID must not be empty")
	}
	if id.Hostname == "" {
		t.Error("Hostname must not be empty")
	}
	if id.OS == "" {
		t.Error("OS must not be empty")
	}
	if id.Arch == "" {
		t.Error("Arch must not be empty")
	}
	if id.AgentVersion == "" {
		t.Error("AgentVersion must not be empty")
	}
	if id.CreatedAt.IsZero() {
		t.Error("CreatedAt must not be zero")
	}

	// ServerID 必须以 srv_ 开头
	if len(id.ServerID) < 4 || id.ServerID[:4] != "srv_" {
		t.Errorf("ServerID must start with 'srv_', got %s", id.ServerID)
	}

	// 写入的文件存在且内容匹配
	data, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		t.Fatalf("identity.json not written: %v", err)
	}
	var readBack Identity
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("identity.json not valid JSON: %v", err)
	}
	if readBack.ServerID != id.ServerID {
		t.Error("read-back ServerID mismatch")
	}
}

func TestNewIdentity_Idempotent(t *testing.T) {
	dir := t.TempDir()

	id1, err := New(dir)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	id2, err := New(dir)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}

	// 第二次调用应该返回已存在的 identity，内容完全一致
	if id1.ServerID != id2.ServerID {
		t.Errorf("ServerID changed: %s -> %s", id1.ServerID, id2.ServerID)
	}
	if id1.Hostname != id2.Hostname {
		t.Errorf("Hostname changed: %s -> %s", id1.Hostname, id2.Hostname)
	}
	if id1.AgentVersion != id2.AgentVersion {
		t.Errorf("AgentVersion changed: %s -> %s", id1.AgentVersion, id2.AgentVersion)
	}
	if !id1.CreatedAt.Equal(id2.CreatedAt) {
		t.Errorf("CreatedAt changed: %v -> %v", id1.CreatedAt, id2.CreatedAt)
	}
}

func TestIdentity_Validate(t *testing.T) {
	tests := []struct {
		name string
		id   Identity
		ok   bool
	}{
		{
			name: "valid identity",
			id:   Identity{ServerID: "srv_abc123", Hostname: "prod-01", OS: "linux", Arch: "amd64", AgentVersion: "1.0.0"},
			ok:   true,
		},
		{
			name: "empty ServerID",
			id:   Identity{ServerID: "", Hostname: "prod-01"},
			ok:   false,
		},
		{
			name: "bad ServerID prefix",
			id:   Identity{ServerID: "bad_123", Hostname: "prod-01"},
			ok:   false,
		},
		{
			name: "empty Hostname",
			id:   Identity{ServerID: "srv_abc", Hostname: ""},
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			if tt.ok && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
			if !tt.ok && err == nil {
				t.Error("Validate() = nil, want error")
			}
		})
	}
}
