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

.PHONY: all build run test test-race lint fmt vet clean \
        install install-service uninstall-service \
        completion-bash completion-zsh install-completion-bash install-completion-zsh \
        version release

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

## lint: Run golangci-lint
lint:
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

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
