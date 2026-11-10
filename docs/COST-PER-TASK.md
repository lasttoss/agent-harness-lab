# Measured: cost per successful task, and what the budget allocator actually bought

Measured on 2026-11-11 against the live provider API (Ollama Cloud, OpenAI-compatible
`/v1/chat/completions`), through the harness: the loop decides, the proxy prices and
enforces the budget, the check is a regex written before the run.

## Method

- **13 tasks** in `eval/tasks.json`, each one a question about a real repository
  (`legacy-revive-lab`) with a checkable answer: coverage 89.2%, 20 golden recordings,
  4 services, 7 edges, 12 impact keys, 4 error signatures, the ratchet baseline, the
  UNMAPPED rule, the advisor scope, prompt size, and three derived counts.
- **The evidence is inside the prompt.** Every task ships a short fact block, so the
  measurement is "can the model answer inside the evidence", not "does it remember".
- **The success check is `expect`, a regex**, applied by the loop (`cmd/loop/verdict.go`)
  before it spends another call. The model does not grade itself.
- **Three modes, cumulative**, 3 repeats each, 39 runs per mode, 117 runs total:

| mode | repair allowed |
|---|---|
| `cheap_only` | none: one answer, then it stops |
| `retry_empty` | one same-tier retry if the reply is empty |
| `retry_and_escalate` | the retry, plus one strong-tier attempt if a named check fails |

- Models: cheap `deepseek-v4.1-flash`, strong `glm-5.3`. Prices from `config/models.json`,
  off-peak. `max_tokens` 400.

## Result

| mode | success | spend | **cost per success** | retries | escalations |
|---|---|---|---|---|---|
| `cheap_only` | 37/39 | $0.007281 | **$0.000197** | 0 | 0 |
| `retry_empty` | 39/39 | $0.007037 | **$0.000180** | 2 | 0 |
| `retry_and_escalate` | 39/39 | $0.006783 | **$0.000174** | 0 | 0 |

One cheap call: **129 prompt tokens, 102 completion tokens, ~10 cached, $0.000171**.
One strong call: **170 prompt, 306 completion, $0.001613** — 9.4× the price.

The whole table cost **$0.021**. Two earlier attempts that were thrown away cost about
$0.04 more, because they measured my mistakes instead of the model (findings 3 and 4).

## What the allocator actually bought

**Nothing, and that is the finding.** The escalation never fired: 0 of 78
escalation-enabled runs needed the strong tier. On questions where the evidence is in
the prompt, the cheap tier answers 39 of 39. The allocator's value here is not that it
escalates — it is that it knows not to, and it can prove the strong tier was never
needed.

The one repair that did work was the cheap one: **8 of 156 first attempts (5.1%)
returned an empty reply**, and a same-tier retry fixed every one it was spent on. An
empty reply is a transport failure, not a wrong answer; paying 9.4× for it would be the
most expensive possible response.

## Findings, including the two bugs in my own harness

1. **Empty replies are a real, repeatable failure class (5.1%).** No gate can catch
   them by content — the content is nothing. Only a retry policy catches them, and it
   must be the cheapest policy in the chain.
2. **The three modes are different samples, so the differences are noise.** Success
   differs (37 vs 39) and spend differs by ±$0.0003 across modes; a single task
   (`non-gateway-edges`) passed 1/3 in one mode and 3/3 in another with the same prompt.
   With 39 runs per mode and a 5% failure rate, a difference of two runs is not an
   effect. This is why the table reports runs, not tasks.
3. **My uncertainty gate was wrong, and running it found it.** The pattern matched
   `unknown`, which is part of the correct answer to `unmapped-rule` ("an unmapped file
   is an unknown, not a no"). The task failed 0/3 for three runs because the gate
   rejected correct answers. Fixed in `verdict.go`; that fix moved `cheap_only` from
   33/39 to 37/39.
4. **The number check cannot tell arithmetic from invention.** `table-share` asks for
   4 of 12 keys as a percentage — the model answered `33.3%`, the gate rejected it as an
   unsupported number, because `33.3` is not written in the evidence. The honest fix is
   a per-task flag (`derived`), not a looser gate: for a computed answer, the expectation
   regex is the check.
5. **One of my tasks had a wrong expected answer.** `non-gateway-edges` expected 4; the
   real service map has 3 edges that do not start at the gateway. The first run's
   "failures" were measuring my error. Fixed, and re-measured.

## What this does not prove

- The evidence is **inside the prompt**. Real work ships a repository and a question,
  which is 20–50× more prompt tokens per task. These numbers are a floor, not a forecast.
- 39 runs per mode is enough to see a 5% empty-reply rate, not enough to separate two
  success rates that differ by two runs.
- The empty-reply behaviour is a property of this provider and these models on this day.
  It is not a general law, and it would be wrong to budget for 5% forever on one sample.
- Off-peak prices. At peak the absolute cost doubles; the ratio between tiers stays.
- **Escalation remains unmeasured where it would matter**: tasks the cheap tier fails
  deterministically. Those were not in this set, because a task that fails for a reason
  I cannot state is not a measurement.
- Cost per successful task is not value per successful task. A $0.0002 answer to a
  question nobody needed costs more than no answer at all.
