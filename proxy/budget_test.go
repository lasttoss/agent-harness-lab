package main

import (
	"path/filepath"
	"testing"
)

func TestLedger_BlocksWhenBudgetExceeded(t *testing.T) {
	l := LoadLedger(filepath.Join(t.TempDir(), "budget.json"), 100)
	if err := l.Check(60); err != nil {
		t.Fatalf("first request should pass: %v", err)
	}
	l.Charge(0.001, 60)
	if err := l.Check(60); err == nil {
		t.Fatal("second request must be blocked")
	}
}

func TestCost_SeparatesCachedInput(t *testing.T) {
	p := Prices{InputPerMTok: 1, CachedInputPerMTok: 0.1, OutputPerMTok: 2}
	full := Cost(p, 1_000_000, 0, 0)
	cached := Cost(p, 1_000_000, 1_000_000, 0)
	if full != 1.0 || cached != 0.1 {
		t.Fatalf("unexpected cost: full=%v cached=%v", full, cached)
	}
}
