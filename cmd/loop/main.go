// Command loop is the smallest possible layer-1 runtime: it owns the prompt, the
// step count, the per-step token ceiling, the stop reason and — since the budget
// allocator landed — which model each step is allowed to use.
//
// Everything it sends goes through the layer-2 proxy, so cost is attributed by the
// proxy, not guessed by the loop.
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
	Model            string  `json:"model"`
	Tier             string  `json:"tier"`
	PromptTokens     int     `json:"prompt_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Reply            string  `json:"reply"`
	Verdict          string  `json:"verdict"`
	CostUSD          float64 `json:"cost_usd"`
}

type runRecord struct {
	Task         string       `json:"task"`
	Model        string       `json:"model"`
	ModelStrong  string       `json:"model_strong,omitempty"`
	Escalate     bool         `json:"escalate"`
	Steps        []stepRecord `json:"steps"`
	StopReason   string       `json:"stop_reason"`
	TotalTokens  int          `json:"total_tokens"`
	TotalCostUSD float64      `json:"total_cost_usd"`
	FirstPassOK  bool         `json:"first_pass_ok"`
	Succeeded    bool         `json:"succeeded"`
	Escalations  int          `json:"escalations"`
	Retries      int          `json:"retries"`
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
	model := flag.String("model", "fixture", "model id for the cheap tier")
	strong := flag.String("model-strong", "", "model id for the strong tier; empty disables escalation")
	escalate := flag.Bool("escalate", false, "retry a failed answer once on the strong tier")
	retryEmpty := flag.Bool("retry-empty", false, "retry once on the same tier when the reply is empty")
	evidence := flag.String("evidence", "", "file with the facts the answer must stay inside")
	expect := flag.String("expect", "", "regex the successful answer must match")
	derived := flag.Bool("derived", false, "the answer is computed from the evidence, so an absent number is not proof of invention")
	out := flag.String("out", "out/loop.json", "where to write the run record")
	flag.Parse()

	ev := ""
	if *evidence != "" {
		b, err := os.ReadFile(*evidence)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read evidence:", err)
			os.Exit(2)
		}
		ev = string(b)
	}

	rec := runRecord{
		Task:         *task,
		Model:        *model,
		ModelStrong:  *strong,
		Escalate:     *escalate && *strong != "",
		StartedUTC:   time.Now().UTC().Format(time.RFC3339),
		ProxyAddress: *proxy,
		PrefixStable: true,
	}
	history := []msg{{Role: "system", Content: "You are a terse engineering assistant. Answer only with facts present in the task. Reply DONE when the task is complete."}}
	if ev != "" {
		history = append(history, msg{Role: "system", Content: "EVIDENCE:\n" + ev})
	}
	history = append(history, msg{Role: "user", Content: *task})

	client := &http.Client{Timeout: 120 * time.Second}
	for i := 1; i <= *steps; i++ {
		stepModel, tier := *model, "cheap"
		body, _ := json.Marshal(reqBody{Model: stepModel, Messages: history})
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
		reply := rb.Choices[0].Message.Content
		v := judgeWith(ev, reply, *expect, *derived)
		rec.Steps = append(rec.Steps, stepRecord{
			Step:             i,
			Model:            stepModel,
			Tier:             tier,
			PromptTokens:     rb.Usage.PromptTokens,
			CacheReadTokens:  rb.Usage.CacheReadInputTokens,
			CompletionTokens: rb.Usage.CompletionTokens,
			Reply:            reply,
			Verdict:          v.Reason,
			CostUSD:          headerFloat(resp.Header.Get("X-Harness-Cost-Usd")),
		})

		if st := tokensOf(rb.Usage); st > *maxStepTokens {
			rec.StopReason = fmt.Sprintf("step_budget: step %d used %d tokens (limit %d)", i, st, *maxStepTokens)
			break
		}
		if tt := tokensSoFar(rec.Steps); tt > *maxTotalTokens {
			rec.StopReason = fmt.Sprintf("total_budget: %d tokens (limit %d)", tt, *maxTotalTokens)
			break
		}

		if v.OK {
			rec.Succeeded = true
			rec.FirstPassOK = rec.Escalations == 0
			rec.StopReason = "task_done"
			break
		}

		// Cheapest repair first: an empty reply is a transport-level failure, not a
		// wrong answer, and paying the strong tier for it is wasted money.
		if *retryEmpty && v.Reason == "empty_reply" && rec.Retries == 0 && i < *steps {
			rec.Retries++
			history = append(history,
				msg{Role: "assistant", Content: "(empty reply)"},
				msg{Role: "user", Content: "Your previous reply was empty. Answer the question in one sentence, using only the evidence."},
			)
			body3, _ := json.Marshal(reqBody{Model: stepModel, Messages: history})
			resp3, err := client.Post(*proxy+"/v1/chat/completions", "application/json", bytes.NewReader(body3))
			if err != nil {
				rec.StopReason = "transport_error_retry: " + err.Error()
				break
			}
			var rb3 respBody
			err = json.NewDecoder(resp3.Body).Decode(&rb3)
			resp3.Body.Close()
			if err != nil || len(rb3.Choices) == 0 {
				rec.StopReason = fmt.Sprintf("bad_response_retry: status=%d", resp3.StatusCode)
				break
			}
			reply3 := rb3.Choices[0].Message.Content
			v3 := judgeWith(ev, reply3, *expect, *derived)
			i++
			rec.Steps = append(rec.Steps, stepRecord{
				Step:             i,
				Model:            stepModel,
				Tier:             "cheap_retry",
				PromptTokens:     rb3.Usage.PromptTokens,
				CacheReadTokens:  rb3.Usage.CacheReadInputTokens,
				CompletionTokens: rb3.Usage.CompletionTokens,
				Reply:            reply3,
				Verdict:          v3.Reason,
				CostUSD:          headerFloat(resp3.Header.Get("X-Harness-Cost-Usd")),
			})
			history = append(history, msg{Role: "assistant", Content: reply3})
			if v3.OK {
				rec.Succeeded = true
				rec.StopReason = "task_done_after_retry"
				break
			}
			if i == *steps {
				rec.StopReason = "max_steps: " + v3.Reason
				break
			}
			v = v3
		}

		// The allocator: one escalation, only when the cheap answer failed a check
		// we can name, and only if a strong tier exists. No silent retries.
		if rec.Escalate && rec.Escalations == 0 && i < *steps {
			rec.Escalations++
			history = append(history,
				msg{Role: "assistant", Content: reply},
				msg{Role: "user", Content: "That answer failed a check: " + v.Reason + ". Answer again using only the evidence. Quote the exact numbers."},
			)
			body2, _ := json.Marshal(reqBody{Model: *strong, Messages: history})
			resp2, err := client.Post(*proxy+"/v1/chat/completions", "application/json", bytes.NewReader(body2))
			if err != nil {
				rec.StopReason = "transport_error_escalation: " + err.Error()
				break
			}
			var rb2 respBody
			err = json.NewDecoder(resp2.Body).Decode(&rb2)
			resp2.Body.Close()
			if err != nil || len(rb2.Choices) == 0 {
				rec.StopReason = fmt.Sprintf("bad_response_escalation: status=%d", resp2.StatusCode)
				break
			}
			if resp2.Header.Get("X-Harness-Prefix-Stable") == "false" {
				rec.PrefixStable = false
			}
			reply2 := rb2.Choices[0].Message.Content
			v2 := judgeWith(ev, reply2, *expect, *derived)
			i++ // the escalation consumed a step
			rec.Steps = append(rec.Steps, stepRecord{
				Step:             i,
				Model:            *strong,
				Tier:             "strong",
				PromptTokens:     rb2.Usage.PromptTokens,
				CacheReadTokens:  rb2.Usage.CacheReadInputTokens,
				CompletionTokens: rb2.Usage.CompletionTokens,
				Reply:            reply2,
				Verdict:          v2.Reason,
				CostUSD:          headerFloat(resp2.Header.Get("X-Harness-Cost-Usd")),
			})
			if st := tokensOf(rb2.Usage); st > *maxStepTokens {
				rec.StopReason = fmt.Sprintf("step_budget: step %d used %d tokens (limit %d)", i, st, *maxStepTokens)
				break
			}
			if tt := tokensSoFar(rec.Steps); tt > *maxTotalTokens {
				rec.StopReason = fmt.Sprintf("total_budget: %d tokens (limit %d)", tt, *maxTotalTokens)
				break
			}
			if v2.OK {
				rec.Succeeded = true
				rec.StopReason = "task_done_after_escalation"
				break
			}
			history = append(history, msg{Role: "assistant", Content: reply2})
			if i == *steps {
				rec.StopReason = "max_steps: " + v2.Reason
				break
			}
			continue
		}

		history = append(history, msg{Role: "assistant", Content: reply})
		if i == *steps {
			rec.StopReason = "max_steps: " + v.Reason
		}
	}

	for _, s := range rec.Steps {
		rec.TotalTokens += s.PromptTokens + s.CompletionTokens
		rec.TotalCostUSD += s.CostUSD
	}
	rec.TotalCostUSD = round(rec.TotalCostUSD)
	if rec.StopReason == "" {
		rec.StopReason = "incomplete"
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

	fmt.Printf("steps=%d tokens=%d cost_usd=%.8f succeeded=%v retries=%d escalations=%d prefix_stable=%v stop=%s\n",
		len(rec.Steps), rec.TotalTokens, rec.TotalCostUSD, rec.Succeeded, rec.Retries, rec.Escalations, rec.PrefixStable, rec.StopReason)
	for _, s := range rec.Steps {
		fmt.Printf("  step %d [%s] prompt=%d cache_read=%d completion=%d verdict=%s\n",
			s.Step, s.Tier, s.PromptTokens, s.CacheReadTokens, s.CompletionTokens, s.Verdict)
	}
	if len(rec.Steps) == 0 {
		os.Exit(1)
	}
}

func tokensOf(u usage) int { return u.PromptTokens + u.CompletionTokens }

func tokensSoFar(steps []stepRecord) int {
	n := 0
	for _, s := range steps {
		n += s.PromptTokens + s.CompletionTokens
	}
	return n
}

func headerFloat(v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func round(f float64) float64 { return float64(int64(f*1e8+0.5)) / 1e8 }
