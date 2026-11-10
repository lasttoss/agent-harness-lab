package main

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	in := "key=sk-abcdef123456789 mail=dung@example.com phone=0912345678"
	got := Redact(in)
	for _, banned := range []string{"sk-abcdef123456789", "dung@example.com", "0912345678"} {
		if strings.Contains(got, banned) {
			t.Fatalf("%q leaked: %s", banned, got)
		}
	}
}
