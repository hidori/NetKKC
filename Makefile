.DEFAULT_GOAL := build

# Prefer a working Go on PATH; the cloud image's /usr/bin/go is a different tool.
CLOUD_GO := $(wildcard /workspace/.netkkc/toolchains/go/bin/go)
GO ?= $(if $(shell go version 2>/dev/null),go,$(or $(CLOUD_GO),go))

ifneq ($(filter $(CLOUD_GO),$(GO)),)
export GOPATH ?= /workspace/.netkkc/gopath
export GOMODCACHE ?= /workspace/.netkkc/gomod
export GOCACHE ?= /workspace/.netkkc/gocache
endif

OAPI_CODEGEN_VERSION := v2.8.0

.PHONY: up down generate build lint lint-fix test test-integration clean

# Run on the host (WSL), not in the dev container.
up:
	docker compose up -d

down:
	docker compose down

generate:
	@mkdir -p internal/api
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config oapi-codegen.yaml openapi.yaml

build:
	@mkdir -p bin
	$(GO) build -o bin/kkc-server .

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

test:
	$(GO) test ./...

# Requires NETKKC_TEST_MOZC_PROFILE (set in the dev container).
test-integration:
	$(GO) test -count=1 ./...

clean:
	$(GO) clean .
	rm -f bin/kkc-server
