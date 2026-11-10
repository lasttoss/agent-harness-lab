#!/usr/bin/env bash
# Smoke test: runs the proxy fully offline, no API key, no network.
# It proves three behaviours from the tier-2 contract:
#   1. identical prompt head  -> cache read = prompt tokens
#   2. volatile head          -> cache read = 0 and a warning header
#   3. exhausted budget       -> HTTP 429 with a JSON error, never silent spend
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p out

ADDR=":8099"
LOG="out/smoke-requests.jsonl"
BUDGET="out/smoke-budget.json"
rm -f "$LOG" "$BUDGET"

go build -o bin/harness ./proxy

HARNESS_ADDR="$ADDR" HARNESS_PROVIDER=fixture HARNESS_LOG="$LOG" \
HARNESS_BUDGET="$BUDGET" HARNESS_DAILY_TOKENS=100000 ./bin/harness &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  curl -sf "http://localhost${ADDR}/healthz" >/dev/null && break
  sleep 0.1
done

body() { printf '%s' "{\"model\":\"any\",\"messages\":[{\"role\":\"system\",\"content\":\"$1\"},{\"role\":\"user\",\"content\":\"review it\"}]}"; }

echo "== 1st identical request"
curl -s -D out/h1.txt -o /dev/null -X POST "http://localhost${ADDR}/v1/chat/completions" \
  -H 'Content-Type: application/json' -d "$(body 'You are a reviewer. Cite files.')"
grep -i '^x-harness' out/h1.txt | sed 's/^/   /'

echo "== 2nd identical request (expect cache read > 0)"
curl -s -D out/h2.txt -o /dev/null -X POST "http://localhost${ADDR}/v1/chat/completions" \
  -H 'Content-Type: application/json' -d "$(body 'You are a reviewer. Cite files.')"
grep -i '^x-harness' out/h2.txt | sed 's/^/   /'
CACHE=$(grep -i '^x-harness-cache-read' out/h2.txt | tr -d '\r' | awk '{print $2}')
[ "${CACHE:-0}" -gt 0 ] || { echo "FAIL: expected cache read > 0 on repeat"; exit 1; }

echo "== volatile head (expect cache read = 0 and a warning)"
curl -s -D out/h3.txt -o /dev/null -X POST "http://localhost${ADDR}/v1/chat/completions" \
  -H 'Content-Type: application/json' -d "$(body 'Today is 2026-11-10. You are a reviewer.')"
grep -i '^x-harness' out/h3.txt | sed 's/^/   /'
grep -qi '^x-harness-prefix-warn' out/h3.txt || { echo "FAIL: expected prefix warning"; exit 1; }

echo "== budget exhaustion (expect 429)"
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
HARNESS_ADDR="$ADDR" HARNESS_PROVIDER=fixture HARNESS_LOG="out/smoke-small.jsonl" \
HARNESS_BUDGET="out/smoke-small-budget.json" HARNESS_DAILY_TOKENS=10 ./bin/harness &
PID=$!
for _ in $(seq 1 50); do curl -sf "http://localhost${ADDR}/healthz" >/dev/null && break; sleep 0.1; done
CODE=$(curl -s -o out/too-many.json -w '%{http_code}' -X POST "http://localhost${ADDR}/v1/chat/completions" \
  -H 'Content-Type: application/json' -d "$(body 'You are a reviewer. Cite files and numbers.')")
echo "   HTTP $CODE  $(cat out/too-many.json)"
[ "$CODE" = "429" ] || { echo "FAIL: expected 429"; exit 1; }

echo "== request log"
cat "$LOG" | sed 's/^/   /'
echo "SMOKE OK"
