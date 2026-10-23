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
MOZC_STATE_DIR ?= $(if $(CLOUD_GO),/workspace/.netkkc/docker-mozc,$(CURDIR)/.local/mozc)
export MOZC_STATE_DIR
export MOZC_PROFILE_DIR ?= $(MOZC_STATE_DIR)/mozc
export MOZC_DOCKER_IMAGE ?= netkkc-mozc:dev
export MOZC_DOCKER_CONTAINER ?= netkkc-mozc-$(shell id -u)

.PHONY: generate build test run clean mozc-build mozc-up mozc-down test-integration

generate:
	@mkdir -p internal/api
	$(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -config oapi-codegen.yaml openapi.yaml

build:
	@mkdir -p bin
	$(GO) build -o bin/kkc-server .

test:
	$(GO) test ./...

mozc-build:
	bash scripts/mozc-docker.sh build

mozc-up: build mozc-build
	bash scripts/mozc-docker.sh start
	./bin/kkc-server -check-mozc -mozc-profile "$(MOZC_PROFILE_DIR)"

mozc-down:
	bash scripts/mozc-docker.sh stop

test-integration: mozc-up
	NETKKC_TEST_MOZC_PROFILE="$(MOZC_PROFILE_DIR)" $(GO) test -count=1 ./...

run: mozc-up
	./bin/kkc-server

clean:
	$(GO) clean .
	rm -f bin/kkc-server
