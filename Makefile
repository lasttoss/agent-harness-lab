.PHONY: help build test smoke eval loop demo-loop gates run clean cost-eval cost-eval-real

help:
	@echo "build   build the proxy binary"
	@echo "test    run unit tests (offline)"
	@echo "smoke   run end-to-end smoke test (offline, no API key)"
	@echo "eval    run the gates against accept/reject cases"
	@echo "gates   alias for eval"
	@echo "demo-loop  run layer-1 loop against layer-2 proxy (offline)"
	@echo "cost-eval  cost per successful task, fixture mode (plumbing only, offline)"
	@echo "cost-eval-real  same measurement against the live provider (needs HARNESS_UPSTREAM_URL + key)"
	@echo "run     run the proxy in fixture mode"
	@echo "clean   remove build output"

build:
	go build -o bin/harness ./proxy
	go build -o bin/loop ./cmd/loop

test:
	go test ./... -v

smoke: build
	./scripts/smoke.sh

eval gates:
	python3 eval/run_eval.py

demo-loop: build
	./scripts/demo-loop.sh

cost-eval: build
	python3 eval/run_cost_eval.py --mode fixture

cost-eval-real: build
	python3 eval/run_cost_eval.py --mode upstream --out out/cost-eval-real.json

run: build
	HARNESS_PROVIDER=fixture ./bin/harness

clean:
	rm -rf bin out
