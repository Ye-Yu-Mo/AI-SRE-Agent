package deploy

import (
	"testing"
)

// 危险配置必须让 ValidateCompose 返回 Valid:false，部署在 apply 阶段被拦截。
// 静默 log 然后继续部署，等于安全检查不存在。
func TestValidateCompose_BlocksDangerousConfig(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		wantValid bool
		wantRisk  string
	}{
		{
			name:      "privileged container",
			content:   "services:\n  app:\n    privileged: true",
			wantValid: false,
			wantRisk:  "privileged",
		},
		{
			name:      "docker.sock mount",
			content:   "services:\n  app:\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock",
			wantValid: false,
			wantRisk:  "docker.sock",
		},
		{
			name:      "root filesystem mount",
			content:   "services:\n  app:\n    volumes:\n      - /:/host",
			wantValid: false,
			wantRisk:  "root filesystem",
		},
		{
			name:      "host network mode",
			content:   "services:\n  app:\n    network_mode: host",
			wantValid: false,
			wantRisk:  "host network",
		},
		{
			name:      "clean config",
			content:   "services:\n  web:\n    image: nginx\n    ports:\n      - 80:80",
			wantValid: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(dir, "docker-compose.yml", tc.content)

			r := ValidateCompose(dir, "docker-compose.yml")

			if r.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v (risks: %v)", r.Valid, tc.wantValid, r.Risks)
			}
			if tc.wantValid && len(r.Risks) > 0 {
				t.Errorf("clean config should have no risks, got %v", r.Risks)
			}
			if !tc.wantValid && len(r.Risks) == 0 {
				t.Error("dangerous config must report at least one risk")
			}
		})
	}
}

// 读不到文件时 Valid:false，且不 panic。
func TestValidateCompose_MissingFile(t *testing.T) {
	dir := t.TempDir()
	r := ValidateCompose(dir, "nonexistent.yml")
	if r.Valid {
		t.Error("missing compose file must not be Valid")
	}
}
