# Measured: what prompt caching actually saves, per call

Measured on 2026-11-10 against the live provider API (Ollama Cloud, OpenAI-compatible
`/v1/chat/completions`), not against a fixture. Prices read from `ollama.com/pricing`
on the same day, off-peak tier (before 12:00 UTC on a weekday).

## Method

- One stable system prompt of ~3,700 tokens: a fixed rule block plus 60 repetitions of a
  paragraph. No dates, no ids, no paths. This is the shape a real reviewer prompt has.
- Three calls per model, same 40-token answer budget:
  1. **cold** — first call, nothing cached;
  2. **warm** — byte-identical repeat;
  3. **volatile** — the same prompt with `Report generated at 2026-11-10T09:20:00Z.`
     prepended, i.e. exactly what the prefix guard exists to catch.
- Cached tokens read from `usage.prompt_tokens_details.cached_tokens`.

## Result

| model | prompt tokens | cold cached | warm cached | volatile cached | cost cold | cost warm | saved per repeat |
|---|---|---|---|---|---|---|---|
| deepseek-v4.1-flash | 3734 | 0 | **3712** | 0 | $0.000584 | $0.000038 | **93.4%** |
| glm-5.3 | 3717 | 0 | **3712** | 0 | $0.005380 | $0.001148 | **78.7%** |
| glm-5.3-flash | 3717 | 0 | **3648** | 0 | $0.000578 | $0.000140 | **75.8%** |

Two facts matter more than the percentages:

1. On a repeat, the provider marked **98–99% of the prompt as cached** for all three
   models. The disk is not the bottleneck; the price ratio is.
2. Prefixing one timestamp took cached tokens from 3,712 to **0** on every model. The
   guard in `proxy/prefix.go` is not theoretical: the same six patterns are what breaks
   caching in production.

## Why the percentages differ when the cache ratio is the same

Because the discount ratio differs, not the hit ratio:

```
deepseek-v4.1-flash   input $0.15  cached $0.003   cached is 2% of input   -> 93.4%
glm-5.3               input $1.40  cached $0.26    cached is 19% of input  -> 78.7%
glm-5.3-flash         input $0.15  cached $0.03    cached is 20% of input  -> 75.8%
```

A model that looks cheap per token can still be the expensive one per *task*, because
output price and cache discount move independently of input price.

## What this does not prove

- Single sample per model, one day, one provider. Not a benchmark; a measurement.
- Windows and TTLs are provider-side and undocumented here; a repeat after a long gap
  may be a miss.
- Off-peak pricing is used. At peak, deepseek input doubles, so the absolute saving
  doubles with it — the percentage stays roughly the same.
- This measures cost per call, **not** cost per successful task. A wrong answer that
  needs three retries costs three times this table.
