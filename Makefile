BINARY    := darwin-exporter
MODULE    := github.com/timansky/darwin-exporter
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -X $(MODULE)/version.Version=$(VERSION) \
           -X $(MODULE)/version.Commit=$(COMMIT) \
           -X $(MODULE)/version.BuildDate=$(BUILD_DATE)

GO      := go
GOFLAGS :=

ifneq (,$(wildcard .env))
include .env
endif

export GRAFANA_URL GRAFANA_USER GRAFANA_PASS GRAFANA_TOKEN GRAFANA_ORG_TOKENS \
       GRAFANA_ORG_IDS GRAFANA_ORG_ID GRAFANA_DASH_GET_ORGID GRAFANA_DASH_UID GRAFANA_DASH_PATH \
       GRAFANA_SYNC_ARGS

GRAFANA_SYNC_SCRIPT ?= scripts/grafana-dashboard-sync.sh
GRAFANA_ORG_IDS ?= 1
GRAFANA_ORG_ID ?=
ORG_ID ?=
GRAFANA_DASH_GET_ORGID ?= 1
GRAFANA_DASH_UID ?=
GRAFANA_DASH_PATH ?=
GRAFANA_ORG_TOKENS ?=
GRAFANA_SYNC_ARGS ?=

.PHONY: all build run test test-race lint lint-md lint-grafana lint-go fmt vet clean \
        install install-service uninstall-service \
        completion-bash completion-zsh install-completion-bash install-completion-zsh \
        version release put-dash get-dash

all: build

## build: Build the binary with CGo (recommended; enables SMC temperature collector)
build:
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -tags cgo -ldflags "$(LDFLAGS)" -o $(BINARY) .

## run: Run darwin-exporter locally (no config file)
run:
	$(GO) run -ldflags "$(LDFLAGS)" . $(ARGS)

## test: Run all unit tests
test:
	$(GO) test -v ./...

## test-race: Run tests with race detector
test-race:
	$(GO) test -race -v ./...

## lint: Run markdownlint, eslint (Grafana dashboard), and golangci-lint
lint: lint-md lint-grafana lint-go

## lint-md: Run markdownlint
lint-md:
	npm run --silent lint:md

## lint-grafana: Run eslint for Grafana dashboard JSON
lint-grafana:
	npm run --silent lint:grafana

## lint-go: Run golangci-lint
lint-go:
	golangci-lint run ./...

## fmt: Format all Go source files
fmt:
	gofmt -w .

## vet: Run go vet
vet:
	$(GO) vet ./...

## clean: Remove build artifacts
clean:
	rm -f $(BINARY)

## install: Install binary to ~/.local/bin/
install: build
	install -d ~/.local/bin
	install -m 0755 $(BINARY) ~/.local/bin/$(BINARY)
	@echo "Installed to ~/.local/bin/$(BINARY)"

## completion-bash: Print bash completion script to stdout
completion-bash: build
	./$(BINARY) completion bash

## completion-zsh: Print zsh completion script to stdout
completion-zsh: build
	./$(BINARY) completion zsh

## install-completion-bash: Install bash completion for current user
install-completion-bash: build
	install -d ~/.local/share/bash-completion/completions
	./$(BINARY) completion bash > ~/.local/share/bash-completion/completions/$(BINARY)

## install-completion-zsh: Install zsh completion for current user
install-completion-zsh: build
	install -d ~/.zsh/completions
	./$(BINARY) completion zsh > ~/.zsh/completions/_$(BINARY)

## install-service: Install launchd service (LaunchAgent + sudoers for wdutil)
install-service: install
	sudo ~/.local/bin/$(BINARY) service install --type=sudo

## uninstall-service: Stop and remove launchd service
uninstall-service:
	sudo ~/.local/bin/$(BINARY) service uninstall --type=sudo

## version: Print version information
version:
	@echo "darwin-exporter $(VERSION) commit=$(COMMIT) built=$(BUILD_DATE)"

## release: Build stripped release binary
release:
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -tags cgo \
		-ldflags "$(LDFLAGS) -s -w" \
		-o $(BINARY) .

## put-dash: Push Grafana dashboard (ORG_ID or GRAFANA_ORG_ID for single-org override)
put-dash:
	@test -n "$(GRAFANA_URL)" || (echo "GRAFANA_URL is required" >&2; exit 1)
	@test -x "$(GRAFANA_SYNC_SCRIPT)" || (echo "script is not executable: $(GRAFANA_SYNC_SCRIPT)" >&2; exit 1)
	@uid_arg=""; dash_arg=""; org_ids="$(GRAFANA_ORG_IDS)"; \
	if [ -n "$(GRAFANA_ORG_ID)" ]; then org_ids="$(GRAFANA_ORG_ID)"; fi; \
	if [ -n "$(ORG_ID)" ]; then org_ids="$(ORG_ID)"; fi; \
	if [ -n "$(GRAFANA_DASH_UID)" ]; then uid_arg="--uid $(GRAFANA_DASH_UID)"; fi; \
	if [ -n "$(GRAFANA_DASH_PATH)" ]; then dash_arg="--dashboard $(GRAFANA_DASH_PATH)"; fi; \
	"$(GRAFANA_SYNC_SCRIPT)" push \
		--url "$(GRAFANA_URL)" \
		--org-ids "$$org_ids" \
		$$dash_arg \
		$$uid_arg \
		$(GRAFANA_SYNC_ARGS)

## get-dash: Pull dashboard from source org back to local JSON (.env + --uid if file has no uid)
get-dash:
	@test -n "$(GRAFANA_URL)" || (echo "GRAFANA_URL is required" >&2; exit 1)
	@test -x "$(GRAFANA_SYNC_SCRIPT)" || (echo "script is not executable: $(GRAFANA_SYNC_SCRIPT)" >&2; exit 1)
	@uid_arg=""; dash_arg=""; \
	if [ -n "$(GRAFANA_DASH_UID)" ]; then uid_arg="--uid $(GRAFANA_DASH_UID)"; fi; \
	if [ -n "$(GRAFANA_DASH_PATH)" ]; then dash_arg="--dashboard $(GRAFANA_DASH_PATH)"; fi; \
	"$(GRAFANA_SYNC_SCRIPT)" pull \
		--url "$(GRAFANA_URL)" \
		--get-org-id "$(GRAFANA_DASH_GET_ORGID)" \
		$$dash_arg \
		$$uid_arg \
		$(GRAFANA_SYNC_ARGS)

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
