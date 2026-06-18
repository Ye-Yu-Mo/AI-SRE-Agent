package deploy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Release struct {
	ID                string `json:"release_id"`
	AppID             string `json:"app_id"`
	ServerID          string `json:"server_id"`
	Commit            string `json:"commit"`
	Image             string `json:"image"`
	Status            string `json:"status"` // active, failed, archived
	HealthcheckStatus string `json:"healthcheck_status"`
	ComposeSnapshot   string `json:"compose_snapshot,omitempty"` // base64 encoded compose file at deploy time
	PreviousReleaseID string `json:"previous_release_id,omitempty"`
	CreatedAt         string `json:"created_at"`
	ActivatedAt       string `json:"activated_at,omitempty"`
}

type ReleaseStore struct {
	releases []Release
	byID     map[string]*Release
	current  map[string]string // appID → releaseID
}

func NewReleaseStore() *ReleaseStore {
	return &ReleaseStore{
		byID:    make(map[string]*Release),
		current: make(map[string]string),
	}
}

func (rs *ReleaseStore) Create(r Release) {
	r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	cp := r
	rs.releases = append(rs.releases, cp)
	rs.byID[r.ID] = &rs.releases[len(rs.releases)-1]
}

func (rs *ReleaseStore) SaveToDisk(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, r := range rs.releases {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
	}
	return nil
}

func (rs *ReleaseStore) LoadFromDisk(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r Release
		if json.Unmarshal([]byte(line), &r) == nil {
			cp := r
			rs.releases = append(rs.releases, cp)
			rs.byID[r.ID] = &rs.releases[len(rs.releases)-1]
			if r.Status == "active" {
				rs.current[r.AppID] = r.ID
			}
		}
	}
}

func (rs *ReleaseStore) Activate(releaseID string) error {
	r, ok := rs.byID[releaseID]
	if !ok {
		return fmt.Errorf("release %s not found", releaseID)
	}

	// 归档之前的 active release
	if prevID, exists := rs.current[r.AppID]; exists {
		if prev, ok := rs.byID[prevID]; ok {
			prev.Status = "archived"
			r.PreviousReleaseID = prevID
		}
	}

	r.Status = "active"
	r.ActivatedAt = time.Now().UTC().Format(time.RFC3339)
	rs.current[r.AppID] = releaseID
	return nil
}

func (rs *ReleaseStore) MarkFailed(releaseID string) error {
	r, ok := rs.byID[releaseID]
	if !ok {
		return fmt.Errorf("release %s not found", releaseID)
	}
	r.Status = "failed"
	return nil
}

func (rs *ReleaseStore) Get(releaseID string) (*Release, bool) {
	r, ok := rs.byID[releaseID]
	if !ok {
		return nil, false
	}
	cp := *r
	return &cp, true
}

func (rs *ReleaseStore) Current(appID string) (*Release, bool) {
	releaseID, ok := rs.current[appID]
	if !ok {
		return nil, false
	}
	return rs.Get(releaseID)
}

// Rollback 回滚到上一个 release
func Rollback(rs *ReleaseStore, appID, workDir, composeFile string) (*Release, error) {
	current, ok := rs.Current(appID)
	if !ok {
		return nil, fmt.Errorf("no active release for app %s", appID)
	}
	if current.PreviousReleaseID == "" {
		return nil, fmt.Errorf("no previous release to rollback to")
	}

	// 停止当前版本
	if err := rs.MarkFailed(current.ID); err != nil {
		return nil, err
	}
	ComposeDown(context.Background(), workDir, composeFile)

	// 激活上一个版本
	prev, ok := rs.Get(current.PreviousReleaseID)
	if !ok {
		return nil, fmt.Errorf("previous release %s not found", current.PreviousReleaseID)
	}
	if err := rs.Activate(prev.ID); err != nil {
		return nil, err
	}

	ctx := context.Background()

	// git checkout 到上一个 commit
	exec.Command("git", "-C", workDir, "checkout", prev.Commit).Run()

	// 若上一个 release 有 compose 快照，恢复 compose 文件（覆盖 checkout 得到的版本）
	if prev.ComposeSnapshot != "" {
		if data, err := base64.StdEncoding.DecodeString(prev.ComposeSnapshot); err == nil {
			os.WriteFile(filepath.Join(workDir, composeFile), data, 0644)
		}
	}

	// 重新构建并启动
	ComposeBuild(ctx, workDir, composeFile)
	ComposeUp(ctx, workDir, composeFile)

	return prev, nil
}

// CloneRepo 浅克隆 Git 仓库
func CloneRepo(url, branch, destDir string) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, url, destDir)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %v\n%s", err, string(out))
	}
	return nil
}
