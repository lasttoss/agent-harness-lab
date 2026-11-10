// Command loop is the smallest possible layer-1 runtime: it owns the prompt, the
// step count, the per-step token ceiling and the stop reason. Everything it sends
// goes through the layer-2 proxy, so cost is attributed by the proxy, not guessed
// by the loop.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

type msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type reqBody struct {
	Model    string `json:"model"`
	Messages []msg  `json:"messages"`
}

type usage struct {
	PromptTokens         int `json:"prompt_tokens"`
	CompletionTokens     int `json:"completion_tokens"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}

type choice struct {
	Message msg `json:"message"`
}

type respBody struct {
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
}

type stepRecord struct {
	Step             int     `json:"step"`
	PromptTokens     int     `json:"prompt_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Reply            string  `json:"reply"`
	CostUSD          float64 `json:"cost_usd"`
}

type runRecord struct {
	Task         string       `json:"task"`
	Model        string       `json:"model"`
	Steps        []stepRecord `json:"steps"`
	StopReason   string       `json:"stop_reason"`
	TotalTokens  int          `json:"total_tokens"`
	TotalCostUSD float64      `json:"total_cost_usd"`
	StartedUTC   string       `json:"started_utc"`
	ProxyAddress string       `json:"proxy_address"`
	PrefixStable bool         `json:"prefix_stable"`
}

func main() {
	task := flag.String("task", "Summarise the diff in one line.", "task for the agent")
	steps := flag.Int("steps", 4, "maximum number of model calls")
	maxStepTokens := flag.Int("max-step-tokens", 4000, "ceiling per model call")
	maxTotalTokens := flag.Int("max-total-tokens", 12000, "ceiling for the whole run")
	proxy := flag.String("proxy", "http://localhost:8099", "layer-2 proxy base URL")
	model := flag.String("model", "fixture", "model id sent to the proxy")
	out := flag.String("out", "out/loop.json", "where to write the run record")
	flag.Parse()

	rec := runRecord{
		Task:         *task,
		Model:        *model,
		StartedUTC:   time.Now().UTC().Format(time.RFC3339),
		ProxyAddress: *proxy,
		PrefixStable: true,
	}
	history := []msg{{Role: "system", Content: "You are a terse engineering assistant. Reply DONE when the task is complete."}, {Role: "user", Content: *task}}

	client := &http.Client{Timeout: 60 * time.Second}
	for i := 1; i <= *steps; i++ {
		body, _ := json.Marshal(reqBody{Model: *model, Messages: history})
		resp, err := client.Post(*proxy+"/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			rec.StopReason = "transport_error: " + err.Error()
			break
		}
		var rb respBody
		err = json.NewDecoder(resp.Body).Decode(&rb)
		resp.Body.Close()
		if err != nil || len(rb.Choices) == 0 {
			rec.StopReason = fmt.Sprintf("bad_response: status=%d", resp.StatusCode)
			break
		}
		if resp.Header.Get("X-Harness-Prefix-Stable") == "false" {
			rec.PrefixStable = false
		}
		stepTokens := rb.Usage.PromptTokens + rb.Usage.CompletionTokens
		rec.Steps = append(rec.Steps, stepRecord{
			Step:             i,
			PromptTokens:     rb.Usage.PromptTokens,
			CacheReadTokens:  rb.Usage.CacheReadInputTokens,
			CompletionTokens: rb.Usage.CompletionTokens,
			Reply:            rb.Choices[0].Message.Content,
			CostUSD:          headerFloat(resp.Header.Get("X-Harness-Cost-Usd")),
		})
		rec.TotalTokens += stepTokens
		for _, s := range rec.Steps {
			rec.TotalCostUSD += s.CostUSD
		}
		rec.TotalCostUSD = round(rec.TotalCostUSD)

		reply := rb.Choices[0].Message.Content
		history = append(history, msg{Role: "assistant", Content: reply})

		switch {
		case stepTokens > *maxStepTokens:
			rec.StopReason = fmt.Sprintf("step_budget: step %d used %d tokens (limit %d)", i, stepTokens, *maxStepTokens)
		case rec.TotalTokens > *maxTotalTokens:
			rec.StopReason = fmt.Sprintf("total_budget: %d tokens (limit %d)", rec.TotalTokens, *maxTotalTokens)
		case contains(reply, "DONE"):
			rec.StopReason = "task_done"
		case i == *steps:
			rec.StopReason = fmt.Sprintf("max_steps: %d", *steps)
		}
		if rec.StopReason != "" {
			break
		}
	}

	if err := os.MkdirAll("out", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	b, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Printf("steps=%d tokens=%d cost_usd=%.8f prefix_stable=%v stop=%s\n",
		len(rec.Steps), rec.TotalTokens, rec.TotalCostUSD, rec.PrefixStable, rec.StopReason)
	for _, s := range rec.Steps {
		fmt.Printf("  step %d: prompt=%d cache_read=%d completion=%d\n", s.Step, s.PromptTokens, s.CacheReadTokens, s.CompletionTokens)
	}
	if rec.StopReason == "" {
		os.Exit(1)
	}
}

func headerFloat(v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func round(f float64) float64 { return float64(int64(f*1e8+0.5)) / 1e8 }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
