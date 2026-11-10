package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Usage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

var (
	mu         sync.Mutex
	seenPrefix = map[string]bool{}
	ledger     *Ledger
	prices     Prices
	logPath    string
	fixture    string
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var req ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	var sb strings.Builder
	for _, m := range req.Messages {
		sb.WriteString(m.Role + ":" + m.Content + "\n")
	}
	prompt := sb.String()
	promptTokens := len(prompt) / 4
	if promptTokens < 1 {
		promptTokens = 1
	}

	fp, warns := AnalyzePrefix(prompt)

	mu.Lock()
	if err := ledger.Check(promptTokens); err != nil {
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
			"hint":  "raise HARNESS_DAILY_TOKENS, or wait for the next UTC day",
		})
		return
	}
	cached := 0
	if seenPrefix[fp] {
		cached = promptTokens // same prompt head => cache hit (simulated in fixture mode)
	}
	mu.Unlock()

	provider := env("HARNESS_PROVIDER", "fixture")
	var resp ChatResponse
	if provider == "fixture" {
		b, err := os.ReadFile(fixture)
		if err != nil {
			http.Error(w, "fixture missing", http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(b, &resp); err != nil || len(resp.Choices) == 0 {
			http.Error(w, "fixture invalid", http.StatusInternalServerError)
			return
		}
		resp.Model = req.Model
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "upstream mode lands in phase 2"})
		return
	}

	completionTokens := len(resp.Choices[0].Message.Content) / 4
	resp.Usage = Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens, CacheReadInputTokens: cached}
	cost := Cost(prices, promptTokens, cached, completionTokens)

	mu.Lock()
	seenPrefix[fp] = true
	ledger.Charge(cost, promptTokens)
	mu.Unlock()

	AppendLog(logPath, map[string]any{
		"provider":          provider,
		"model":             req.Model,
		"prompt_tokens":     promptTokens,
		"cache_read_tokens": cached,
		"completion_tokens": completionTokens,
		"cost_usd":          cost,
		"prefix_fp":         fp,
		"prefix_warnings":   warns,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Harness-Provider", provider)
	w.Header().Set("X-Harness-Cost-Usd", fmt.Sprintf("%.8f", cost))
	w.Header().Set("X-Harness-Cache-Read", fmt.Sprint(cached))
	w.Header().Set("X-Harness-Prefix-Stable", fmt.Sprint(len(warns) == 0))
	if len(warns) > 0 {
		w.Header().Set("X-Harness-Prefix-Warn", strings.Join(warns, ";"))
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func main() {
	prices = LoadPrices(env("HARNESS_PRICES", "config/prices.json"), env("HARNESS_PRICE_KEY", "default"))
	logPath = env("HARNESS_LOG", "out/requests.jsonl")
	fixture = env("HARNESS_FIXTURE", "proxy/fixtures/response.json")
	daily, _ := strconv.Atoi(env("HARNESS_DAILY_TOKENS", "200000"))
	ledger = LoadLedger(env("HARNESS_BUDGET", "out/budget.json"), daily)

	http.HandleFunc("/v1/chat/completions", handle)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	addr := env("HARNESS_ADDR", ":8099")
	log.Printf("harness proxy listening on %s (provider=%s, daily_tokens=%d)", addr, env("HARNESS_PROVIDER", "fixture"), daily)
	log.Fatal(http.ListenAndServe(addr, nil))
}
