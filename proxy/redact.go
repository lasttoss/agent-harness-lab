package main

import "regexp"

var redactions = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`sk-[A-Za-z0-9_\-]{8,}`), "sk-REDACTED"},
	{regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`), "EMAIL-REDACTED"},
	{regexp.MustCompile(`(?:\+84|0)\d{9,10}\b`), "PHONE-REDACTED"},
}

// Redact removes secrets and personal data before anything is written to a log.
func Redact(s string) string {
	for _, r := range redactions {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}
