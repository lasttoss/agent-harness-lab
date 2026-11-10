# Architecture: three layers, and why each one needs a different owner

The harness is split by *who can change it*, not by technology.

## Layer 1 — inside the agent loop
Owned by: whoever builds the runtime.
Contains: system prompt, tool schemas, retry policy, context compaction,
model choice per step, token budget per step, prefix stabilisation.
A closed CLI does not expose this layer. That is fine for interactive work and
fatal for a product: you cannot attribute cost per step, and you cannot
reproduce behaviour after a vendor upgrade.

## Layer 2 — the tool boundary (this repo's `proxy/`)
Owned by: platform team.
Contains: provider selection and failover, daily token budget, per-request cost
accounting, PII redaction, prefix volatility guard, request log.
Contract: it never changes the meaning of a request. It only observes, limits,
and routes. If it needs business logic, the logic is in the wrong layer.

## Layer 3 — outside the loop (this repo's `gates/`, next phase)
Owned by: whoever owns correctness.
Contains: tests, coverage floor and ratchet, schema checks, "every number must be
sourced" checks, golden recordings.
Runs on CI, not on the agent. This is the only layer that can say "wrong" and be
believed, because it does not depend on the agent telling the truth about itself.

## Why the split matters

- A proxy without gates gives guardrail theatre: dashboards go up, silent wrong
  answers still ship.
- Gates without a proxy give truth but no cost control: correctness is protected,
  the invoice is not.
- A runtime without either is a demo.

## Deliberate non-goals

- No model training, no fine-tuning, no prompt marketplace.
- No hidden retries that mask provider failure.
- No "AI writes the numbers" path anywhere in build.
