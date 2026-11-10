package main

import (
	"encoding/json"
	"os"
)

type Prices struct {
	InputPerMTok       float64 `json:"input_per_mtok"`
	CachedInputPerMTok float64 `json:"cached_input_per_mtok"`
	OutputPerMTok      float64 `json:"output_per_mtok"`
}

func LoadPrices(path, key string) Prices {
	var all map[string]Prices
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &all) != nil {
		return Prices{}
	}
	if p, ok := all[key]; ok {
		return p
	}
	return all["default"]
}

// Cost separates cached input from fresh input and from output, because a model
// that looks cheap per token can still be expensive per successful task.
func Cost(p Prices, promptTokens, cachedTokens, completionTokens int) float64 {
	fresh := promptTokens - cachedTokens
	if fresh < 0 {
		fresh = 0
	}
	return float64(fresh)/1e6*p.InputPerMTok +
		float64(cachedTokens)/1e6*p.CachedInputPerMTok +
		float64(completionTokens)/1e6*p.OutputPerMTok
}
