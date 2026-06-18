package secret

import "testing"

func TestRedactLine_RedactsPasswordURL(t *testing.T) {
	input := "DATABASE_URL=postgres://user:secretpass@localhost:5432/db"
	got := RedactLine(input)
	if got == input {
		t.Error("password in URL was not redacted")
	}
	if contains(got, "secretpass") {
		t.Errorf("raw password still visible: %s", got)
	}
}

func TestRedactLine_RedactsAPIKey(t *testing.T) {
	input := "OPENAI_API_KEY=sk-proj-abc123def456"
	got := RedactLine(input)
	if got == input {
		t.Error("API key was not redacted")
	}
	if contains(got, "abc123") {
		t.Errorf("raw key still visible: %s", got)
	}
}

func TestRedactLine_RedactsPasswordKey(t *testing.T) {
	cases := []string{
		"PASSWORD=supersecret",
		"DB_PASSWORD=supersecret",
		"POSTGRES_PASSWORD=supersecret",
		"MYSQL_ROOT_PASSWORD=supersecret",
	}
	for _, input := range cases {
		got := RedactLine(input)
		if got == input {
			t.Errorf("%q was not redacted", input)
		}
		if contains(got, "supersecret") {
			t.Errorf("%q raw secret leaked: %s", input, got)
		}
	}
}

func TestRedactLine_RedactsToken(t *testing.T) {
	input := "AUTH_TOKEN=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOn0.signature"
	got := RedactLine(input)
	if got == input {
		t.Error("token was not redacted")
	}
}

func TestRedactLine_PreservesNonSecret(t *testing.T) {
	lines := []string{
		"CPU: 23.5%",
		"Memory: 1.5GB / 4GB",
		"nginx.service active running",
		"myapp_web_1 Up 2 hours",
	}
	for _, line := range lines {
		got := RedactLine(line)
		if got != line {
			t.Errorf("non-secret line was modified: %q → %q", line, got)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
