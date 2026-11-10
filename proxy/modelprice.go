package main

import (
	"encoding/json"
	"os"
)

// ModelPrice is a price row plus the tier the loop's allocator may use.
type ModelPrice struct {
	Tier               string  `json:"tier"`
	InputPerMTok       float64 `json:"input_per_mtok"`
	CachedInputPerMTok float64 `json:"cached_input_per_mtok"`
	OutputPerMTok      float64 `json:"output_per_mtok"`
	MeasuredColdUSD    float64 `json:"measured_cold_usd"`
	MeasuredPromptTok  int     `json:"measured_prompt_tokens"`
}

func (m ModelPrice) Prices() Prices {
	return Prices{InputPerMTok: m.InputPerMTok, CachedInputPerMTok: m.CachedInputPerMTok, OutputPerMTok: m.OutputPerMTok}
}

// LoadModelPrices reads the per-model table. A missing or malformed file is not
// fatal: the caller falls back to the flat price key so the proxy still runs.
func LoadModelPrices(path string) map[string]ModelPrice {
	var all map[string]ModelPrice
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if json.Unmarshal(b, &all) != nil {
		return nil
	}
	delete(all, "_note")
	return all
}

// PriceFor resolves the price of one model id. Order: exact match, then the flat
// key from config/prices.json, and finally the flat default. An unknown model is
// priced as the default, never as free: a silent zero cost is a worse bug than an
// approximate one.
func PriceFor(byModel map[string]ModelPrice, flat Prices, model string) Prices {
	if p, ok := byModel[model]; ok {
		return p.Prices()
	}
	return flat
}
