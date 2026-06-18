package deploy

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Release 能存取 ComposeSnapshot，且 JSON 往返保留该字段。
func TestRelease_ComposeSnapshot(t *testing.T) {
	rs := NewReleaseStore()
	snap := base64.StdEncoding.EncodeToString([]byte("services:\n  web:\n    image: nginx"))
	rs.Create(Release{ID: "rel_1", AppID: "app", Status: "active", ComposeSnapshot: snap})

	got, ok := rs.Get("rel_1")
	if !ok {
		t.Fatal("release not found")
	}
	if got.ComposeSnapshot != snap {
		t.Errorf("ComposeSnapshot = %q, want %q", got.ComposeSnapshot, snap)
	}

	// JSON 往返
	b, _ := json.Marshal(got)
	var back Release
	json.Unmarshal(b, &back)
	if back.ComposeSnapshot != snap {
		t.Error("ComposeSnapshot lost in JSON round-trip")
	}
}

// 旧数据（无 compose_snapshot 字段）反序列化不崩溃，字段为空。
func TestRelease_BackwardCompatNoSnapshot(t *testing.T) {
	old := `{"release_id":"rel_old","app_id":"app","status":"active","commit":"abc"}`
	var r Release
	if err := json.Unmarshal([]byte(old), &r); err != nil {
		t.Fatalf("unmarshal old record: %v", err)
	}
	if r.ComposeSnapshot != "" {
		t.Errorf("expected empty snapshot for old record, got %q", r.ComposeSnapshot)
	}
}

// Rollback 时，若 prev release 有快照，应把 compose 文件写回为快照内容。
func TestRollback_RestoresComposeSnapshot(t *testing.T) {
	dir := t.TempDir()
	composeFile := "docker-compose.yml"
	// 当前 compose 文件内容（新版本）
	os.WriteFile(filepath.Join(dir, composeFile), []byte("NEW VERSION"), 0644)
	// 初始化 git repo 让 checkout 不报致命错（rollback 内部调 git checkout）
	gitInit(t, dir)

	rs := NewReleaseStore()
	oldSnap := base64.StdEncoding.EncodeToString([]byte("OLD VERSION"))
	rs.Create(Release{ID: "rel_old", AppID: "app", Status: "active", Commit: "HEAD", ComposeSnapshot: oldSnap})
	rs.Activate("rel_old")
	rs.Create(Release{ID: "rel_new", AppID: "app", Status: "active", Commit: "HEAD"})
	rs.Activate("rel_new") // rel_new.PreviousReleaseID = rel_old

	_, err := Rollback(rs, "app", dir, composeFile)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, composeFile))
	if string(content) != "OLD VERSION" {
		t.Errorf("compose file = %q, want restored snapshot 'OLD VERSION'", string(content))
	}
}

// Rollback 时 prev release 无快照（旧数据），不写回文件、不崩溃。
func TestRollback_NoSnapshotDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	composeFile := "docker-compose.yml"
	os.WriteFile(filepath.Join(dir, composeFile), []byte("CURRENT"), 0644)
	gitInit(t, dir)

	rs := NewReleaseStore()
	rs.Create(Release{ID: "rel_old", AppID: "app", Status: "active", Commit: "HEAD"})
	rs.Activate("rel_old")
	rs.Create(Release{ID: "rel_new", AppID: "app", Status: "active", Commit: "HEAD"})
	rs.Activate("rel_new")

	_, err := Rollback(rs, "app", dir, composeFile)
	if err != nil {
		t.Fatalf("rollback should not error without snapshot: %v", err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Run()
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	run("add", ".")
	run("commit", "-m", "init")
}
