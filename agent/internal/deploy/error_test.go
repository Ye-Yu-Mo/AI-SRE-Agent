package deploy

import "testing"

func TestTranslateError(t *testing.T) {
	cases := []struct {
		raw       string
		exitCode  int
		wantCode  string
		wantCat   string
	}{
		{"exit status 125", 125, "BUILD_FAILED", "docker"},
		{"exit status 127", 127, "CMD_NOT_FOUND", "system"},
		{"exit status 128", 128, "CLONE_FAILED", "git"},
		{"exit status 137", 137, "OOM_KILLED", "container"},
		{"generic error", 1, "UNKNOWN_ERROR", "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.wantCode, func(t *testing.T) {
			got := TranslateError(tc.exitCode, tc.raw)
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			if got.Category != tc.wantCat {
				t.Errorf("Category = %q, want %q", got.Category, tc.wantCat)
			}
			if got.Message == "" {
				t.Error("Message is empty")
			}
			if got.Raw != tc.raw {
				t.Errorf("Raw = %q, want %q", got.Raw, tc.raw)
			}
		})
	}
}
