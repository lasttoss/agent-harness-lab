package main

import "testing"

// The price table has to stay compatible with the calls that were measured against
// the live provider. The answer length of those calls was not recorded, so the
// honest check is a bracket, not an equality: the table must put the measured cost
// inside the range the same call would cost with a 0-token answer and with a
// 64-token answer. A price row that drifts out of that bracket is wrong.
func TestPricesBracketMeasuredColdCost(t *testing.T) {
	byModel := LoadModelPrices("../config/models.json")
	if len(byModel) == 0 {
		t.Fatal("config/models.json did not load")
	}
	checked := 0
	for name, m := range byModel {
		if m.MeasuredColdUSD == 0 {
			continue
		}
		lo := Cost(m.Prices(), m.MeasuredPromptTok, 0, 0)
		hi := Cost(m.Prices(), m.MeasuredPromptTok, 0, 64)
		if m.MeasuredColdUSD < lo || m.MeasuredColdUSD > hi {
			t.Errorf("%s: measured $%.6f outside [$%.6f, $%.6f] implied by the table", name, m.MeasuredColdUSD, lo, hi)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no price row carries a measurement")
	}
}

func TestUnknownModelIsNotFree(t *testing.T) {
	byModel := LoadModelPrices("../config/models.json")
	flat := Prices{InputPerMTok: 0.15, CachedInputPerMTok: 0.003, OutputPerMTok: 1.5}
	if p := PriceFor(byModel, flat, "model-that-does-not-exist"); p.InputPerMTok == 0 {
		t.Fatal("unknown model priced as free; a silent zero cost hides spend")
	}
}
