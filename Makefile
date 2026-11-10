.PHONY: help build test smoke eval loop demo-loop gates run clean

help:
	@echo "build   build the proxy binary"
	@echo "test    run unit tests (offline)"
	@echo "smoke   run end-to-end smoke test (offline, no API key)"
	@echo "eval    run the gates against accept/reject cases"
	@echo "gates   alias for eval"
	@echo "demo-loop  run layer-1 loop against layer-2 proxy (offline)"
	@echo "run     run the proxy in fixture mode"
	@echo "clean   remove build output"

build:
	go build -o bin/harness ./proxy

test:
	go test ./proxy/ -v

smoke: build
	./scripts/smoke.sh

eval gates:
	python3 eval/run_eval.py

demo-loop: build
	./scripts/demo-loop.sh

run: build
	HARNESS_PROVIDER=fixture ./bin/harness

clean:
	rm -rf bin out
