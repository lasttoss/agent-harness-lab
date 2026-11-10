package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Ledger enforces a hard daily token budget. Nothing is spent silently:
// once the budget is gone the proxy answers 429 instead of forwarding.
type Ledger struct {
	Path        string  `json:"-"`
	Day         string  `json:"day"`
	Tokens      int     `json:"tokens"`
	SpentUSD    float64 `json:"spent_usd"`
	DailyTokens int     `json:"daily_tokens_limit"`
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

func LoadLedger(path string, dailyTokens int) *Ledger {
	l := &Ledger{Path: path, Day: today(), DailyTokens: dailyTokens}
	if b, err := os.ReadFile(path); err == nil {
		var onDisk Ledger
		if json.Unmarshal(b, &onDisk) == nil {
			l.Tokens, l.SpentUSD = onDisk.Tokens, onDisk.SpentUSD
			l.Day = onDisk.Day
			if onDisk.DailyTokens > 0 {
				l.DailyTokens = onDisk.DailyTokens
			}
		}
	}
	if l.Day != today() {
		l.Day, l.Tokens, l.SpentUSD = today(), 0, 0
	}
	return l
}

// Check returns an error when the next request would break the budget.
func (l *Ledger) Check(estimatedTokens int) error {
	if l.DailyTokens > 0 && l.Tokens+estimatedTokens > l.DailyTokens {
		return fmt.Errorf("daily token budget exceeded: used=%d limit=%d", l.Tokens, l.DailyTokens)
	}
	return nil
}

func (l *Ledger) Charge(usd float64, tokens int) {
	l.Tokens += tokens
	l.SpentUSD += usd
	b, _ := json.MarshalIndent(l, "", "  ")
	_ = os.WriteFile(l.Path, b, 0o644)
}
