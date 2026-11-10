package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Upstream is the real provider. The proxy owns the budget and the accounting, so
// the upstream call stays dumb: forward the messages, ask for a small answer, and
// return the usage the provider reports rather than a number we invented.
type Upstream struct {
	URL     string
	Key     string
	MaxTok  int
	Timeout time.Duration
}

type upstreamUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type upstreamResp struct {
	Choices []Choice      `json:"choices"`
	Usage   upstreamUsage `json:"usage"`
}

func LoadUpstream() (*Upstream, error) {
	url := strings.TrimSpace(os.Getenv("HARNESS_UPSTREAM_URL"))
	if url == "" {
		return nil, fmt.Errorf("HARNESS_UPSTREAM_URL is not set")
	}
	key := strings.TrimSpace(os.Getenv("HARNESS_API_KEY"))
	if key == "" {
		if p := strings.TrimSpace(os.Getenv("HARNESS_KEY_FILE")); p != "" {
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("read HARNESS_KEY_FILE: %w", err)
			}
			key = strings.TrimSpace(string(b))
		}
	}
	if key == "" {
		return nil, fmt.Errorf("no API key: set HARNESS_API_KEY or HARNESS_KEY_FILE")
	}
	maxTok := 256
	if v := strings.TrimSpace(os.Getenv("HARNESS_MAX_TOKENS")); v != "" {
		fmt.Sscanf(v, "%d", &maxTok)
	}
	return &Upstream{URL: url, Key: key, MaxTok: maxTok, Timeout: 90 * time.Second}, nil
}

func (u *Upstream) Complete(req ChatRequest) (ChatResponse, upstreamUsage, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      req.Model,
		"messages":   req.Messages,
		"max_tokens": u.MaxTok,
		"stream":     false,
	})
	hreq, err := http.NewRequest(http.MethodPost, u.URL, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, upstreamUsage{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+u.Key)
	client := &http.Client{Timeout: u.Timeout}
	resp, err := client.Do(hreq)
	if err != nil {
		return ChatResponse{}, upstreamUsage{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, upstreamUsage{}, fmt.Errorf("upstream %d: %s", resp.StatusCode, firstBytes(raw, 300))
	}
	var ur upstreamResp
	if err := json.Unmarshal(raw, &ur); err != nil {
		return ChatResponse{}, upstreamUsage{}, fmt.Errorf("upstream json: %w", err)
	}
	if len(ur.Choices) == 0 {
		return ChatResponse{}, upstreamUsage{}, fmt.Errorf("upstream returned no choices")
	}
	out := ChatResponse{Model: req.Model, Choices: ur.Choices}
	return out, ur.Usage, nil
}

func firstBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
