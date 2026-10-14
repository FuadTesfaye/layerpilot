BINARY_NAME=compat
BIN_DIR=bin
GO=go

.PHONY: all build test clean run lint install

all: build test

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags="-s -w -X 'github.com/FuadTesfaye/layerpilot/internal/version.Version=v0.1.0' -X 'github.com/FuadTesfaye/layerpilot/internal/version.GitCommit=$$(git rev-parse --short HEAD 2>/dev/null || echo unknown)' -X 'github.com/FuadTesfaye/layerpilot/internal/version.BuildDate=$$(date -u +'%Y-%m-%dT%H:%M:%SZ')'" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/compat

test:
	$(GO) test -v -race ./...

lint:
	test -z "$$($(GO) fmt ./...)"

install: build
	cp $(BIN_DIR)/$(BINARY_NAME) $(HOME)/.local/bin/$(BINARY_NAME)

clean:
	rm -rf $(BIN_DIR)
