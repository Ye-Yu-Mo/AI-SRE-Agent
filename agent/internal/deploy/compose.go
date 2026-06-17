package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ComposeValidateResult struct {
	Valid bool     `json:"valid"`
	Risks []string `json:"risks,omitempty"`
}

// ValidateCompose 检查 compose 文件是否有危险配置
func ValidateCompose(dir, composeFile string) ComposeValidateResult {
	data, err := os.ReadFile(filepath.Join(dir, composeFile))
	if err != nil {
		return ComposeValidateResult{Valid: false, Risks: []string{fmt.Sprintf("cannot read compose file: %v", err)}}
	}
	content := string(data)
	var risks []string

	if strings.Contains(content, "privileged: true") {
		risks = append(risks, "privileged container detected")
	}
	if strings.Contains(content, "network_mode: host") || strings.Contains(content, "network_mode: \"host\"") {
		risks = append(risks, "host network mode detected")
	}
	if strings.Contains(content, "docker.sock") || strings.Contains(content, "/var/run/docker.sock") {
		risks = append(risks, "docker.sock mount detected — container can control host Docker")
	}
	if strings.Contains(content, "- /:/host") || strings.Contains(content, "- /:/") {
		risks = append(risks, "root filesystem mount detected")
	}

	return ComposeValidateResult{
		Valid: len(risks) == 0,
		Risks: risks,
	}
}

// ComposeBuild 执行 docker-compose build
func ComposeBuild(ctx context.Context, dir, composeFile string) (stdout, stderr string, err error) {
	args := []string{"-f", composeFile, "build"}
	out, err := runCmd(ctx, dir, "docker-compose", args...)
	if err != nil {
		return "", string(out), err
	}
	return string(out), "", nil
}

// ComposeUp 执行 docker-compose up -d
func ComposeUp(ctx context.Context, dir, composeFile string) (stdout, stderr string, err error) {
	args := []string{"-f", composeFile, "up", "-d", "--remove-orphans"}
	out, err := runCmd(ctx, dir, "docker-compose", args...)
	if err != nil {
		return "", string(out), err
	}
	return string(out), "", nil
}

// ComposeDown 执行 docker-compose down
func ComposeDown(ctx context.Context, dir, composeFile string) (stdout, stderr string, err error) {
	args := []string{"-f", composeFile, "down", "--remove-orphans"}
	out, err := runCmd(ctx, dir, "docker-compose", args...)
	if err != nil {
		return "", string(out), err
	}
	return string(out), "", nil
}

var runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
