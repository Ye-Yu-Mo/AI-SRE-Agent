package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_DockerCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(dir, "Dockerfile", "FROM nginx")
	writeFile(dir, "docker-compose.yml", "services:\n  web:\n    build: .")

	r := Detect(dir)
	if r.Runtime != RuntimeDockerCompose {
		t.Errorf("Runtime = %s, want docker_compose", r.Runtime)
	}
	if len(r.Files) < 2 {
		t.Errorf("expected at least 2 files, got %v", r.Files)
	}
}

func TestDetect_NodeApp(t *testing.T) {
	dir := t.TempDir()
	writeFile(dir, "Dockerfile", "FROM node:18")
	writeFile(dir, "package.json", "{}")

	r := Detect(dir)
	if r.Runtime != RuntimeDockerfile {
		t.Errorf("Runtime = %s, want dockerfile", r.Runtime)
	}
}

func TestDetect_DockerfileOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(dir, "Dockerfile", "FROM alpine")

	r := Detect(dir)
	if r.Runtime != RuntimeDockerfile {
		t.Errorf("Runtime = %s, want dockerfile", r.Runtime)
	}
}

func TestDetect_Unknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(dir, "README.md", "# hello")
	writeFile(dir, "main.go", "package main")

	r := Detect(dir)
	if r.Runtime != RuntimeUnknown {
		t.Errorf("Runtime = %s, want unknown", r.Runtime)
	}
}

func TestDetect_ComposeV2Naming(t *testing.T) {
	dir := t.TempDir()
	writeFile(dir, "compose.yaml", "services:\n  app:\n    image: nginx")

	r := Detect(dir)
	if r.Runtime == RuntimeUnknown {
		t.Error("should detect compose.yaml as docker_compose")
	}
}

func writeFile(dir, name, content string) {
	os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
}
