package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AppendLog writes one JSON object per request. Logs are for answers, not for
// decoration: every line can be replayed to rebuild cost and cache behaviour.
func AppendLog(path string, rec map[string]any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
}
