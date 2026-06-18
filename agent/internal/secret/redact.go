package secret

import "regexp"

var redactPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// URL password: postgres://user:PASS@host → postgres://user:****@host
	{regexp.MustCompile(`://([^:]+):([^@]+)@`), `://$1:****@`},
	// API keys: KEY=sk-xxx → KEY=sk-**** (keep prefix, mask value)
	{regexp.MustCompile(`(?i)(api[_-]?key|openai[_-]?api[_-]?key)\s*[:=]\s*\S+`), `${1}=****`},
	// PASSWORD=xxx → PASSWORD=****
	{regexp.MustCompile(`(?i)(password|passwd|pass)\s*[:=]\s*\S+`), `${1}=****`},
	// TOKEN=xxx → TOKEN=****
	{regexp.MustCompile(`(?i)(token|secret|auth)\s*[:=]\s*\S+`), `${1}=****`},
	// --password xxx → --password ****
	{regexp.MustCompile(`(?i)--password\s+\S+`), `--password ****`},
	// --secret xxx → --secret ****
	{regexp.MustCompile(`(?i)--secret\s+\S+`), `--secret ****`},
}

// RedactLine masks secrets in a single line of text.
func RedactLine(line string) string {
	result := line
	for _, p := range redactPatterns {
		result = p.re.ReplaceAllString(result, p.repl)
	}
	return result
}
