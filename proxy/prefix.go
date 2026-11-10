package main

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// PrefixGuard reports whether the head of a prompt contains content that changes
// between requests. Any such content breaks prompt caching from that point on.
var volatilePatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"iso-date", regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)},
	{"clock-time", regexp.MustCompile(`\b\d{2}:\d{2}(:\d{2})?\b`)},
	{"uuid", regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}`)},
	{"git-sha", regexp.MustCompile(`\b[0-9a-f]{40}\b`)},
	{"unix-ts", regexp.MustCompile(`\b1[0-9]{9}\b`)},
	{"abs-path", regexp.MustCompile(`(?:/home/|/Users/|C:\\\\)`)},
}

const guardWindow = 2000 // only the head of the prompt matters for cache hits

// AnalyzePrefix returns a stable fingerprint of the prompt head plus a list of
// volatile markers found inside that window.
func AnalyzePrefix(prompt string) (string, []string) {
	head := prompt
	if len(head) > guardWindow {
		head = head[:guardWindow]
	}
	var warns []string
	for _, p := range volatilePatterns {
		if p.re.MatchString(head) {
			warns = append(warns, p.name)
		}
	}
	norm := strings.Join(strings.Fields(head), " ")
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:8]), warns
}
