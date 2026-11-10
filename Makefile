.PHONY: help build test smoke run clean

help:
	@echo "build   build the proxy binary"
	@echo "test    run unit tests (offline)"
	@echo "smoke   run end-to-end smoke test (offline, no API key)"
	@echo "run     run the proxy in fixture mode"
	@echo "clean   remove build output"

build:
	go build -o bin/harness ./proxy

test:
	go test ./proxy/ -v

smoke: build
	./scripts/smoke.sh

run: build
	HARNESS_PROVIDER=fixture ./bin/harness

clean:
	rm -rf bin out
