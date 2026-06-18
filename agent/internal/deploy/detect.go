package deploy

import (
	"os"
	"path/filepath"
)

type Runtime string

const (
	RuntimeDockerCompose Runtime = "docker_compose"
	RuntimeDockerfile    Runtime = "dockerfile"
	RuntimeUnknown       Runtime = "unknown"
)

type DetectResult struct {
	Runtime Runtime  `json:"runtime"`
	Files   []string `json:"files"`
}

var knownFiles = map[string]struct {
	weight int
	runtime Runtime
}{
	"Dockerfile":        {1, RuntimeDockerfile},
	"docker-compose.yml": {2, RuntimeDockerCompose},
	"compose.yaml":      {2, RuntimeDockerCompose},
	"docker-compose.yaml": {2, RuntimeDockerCompose},
	"package.json":   {1, RuntimeDockerfile},
	"requirements.txt": {1, RuntimeDockerfile},
	"go.mod":          {1, RuntimeDockerfile},
	"Cargo.toml":      {1, RuntimeDockerfile},
	"Makefile":        {1, RuntimeDockerfile},
}

func Detect(dir string) DetectResult {
	r := DetectResult{Runtime: RuntimeUnknown}
	hasDockerfile := false
	for name, info := range knownFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			r.Files = append(r.Files, name)
			if info.weight > 0 && (r.Runtime == RuntimeUnknown || info.weight >= 2) {
				r.Runtime = info.runtime
			}
			if name == "Dockerfile" {
				hasDockerfile = true
			}
		}
	}
	// 根目录没找到 Dockerfile，walk 一层子目录
	if !hasDockerfile {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() || e.Name()[0] == '.' {
				continue
			}
			subDir := filepath.Join(dir, e.Name())
			if _, err := os.Stat(filepath.Join(subDir, "Dockerfile")); err == nil {
				r.Files = append(r.Files, filepath.Join(e.Name(), "Dockerfile"))
				if r.Runtime == RuntimeUnknown {
					r.Runtime = RuntimeDockerfile
				}
				break
			}
		}
	}
	return r
}
