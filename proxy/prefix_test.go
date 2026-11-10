package main

import "testing"

func TestAnalyzePrefix_StableWhenNothingVolatile(t *testing.T) {
	prompt := "system:You are a backend reviewer. Rules: cite files, never invent numbers.\nuser:review this diff\n"
	fp1, w1 := AnalyzePrefix(prompt)
	fp2, w2 := AnalyzePrefix(prompt)
	if fp1 != fp2 {
		t.Fatalf("fingerprint must be stable: %s != %s", fp1, fp2)
	}
	if len(w1) != 0 || len(w2) != 0 {
		t.Fatalf("expected no warnings, got %v %v", w1, w2)
	}
}

func TestAnalyzePrefix_DetectsVolatileHead(t *testing.T) {
	cases := map[string]string{
		"iso-date":   "system:Today is 2026-11-10. Review the diff.",
		"clock-time": "system:Current time 08:21. Review the diff.",
		"uuid":       "system:run 123e4567-e89b-12d3-a456-426614174000 review",
		"git-sha":    "system:commit 4f3b2a1c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a review",
		"unix-ts":    "system:ts 1794242048 review",
		"abs-path":   "system:read /home/dy52337/projects/dungtt/legacy-revive-lab review",
	}
	for want, prompt := range cases {
		_, warns := AnalyzePrefix(prompt)
		found := false
		for _, w := range warns {
			if w == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s not detected in %q (got %v)", want, prompt, warns)
		}
	}
}

func TestAnalyzePrefix_IgnoresTail(t *testing.T) {
	head := "system:stable prefix with no dates. "
	for len(head) < guardWindow+10 {
		head += "x"
	}
	prompt := head + " 2026-11-10"
	_, warns := AnalyzePrefix(prompt)
	if len(warns) != 0 {
		t.Fatalf("volatile content after the guard window must not be reported: %v", warns)
	}
}
