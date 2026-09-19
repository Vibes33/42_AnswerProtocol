GO      ?= go
BIN     := bin
ADDR    ?= localhost:4242
ARGS    ?=

SERVER  := $(BIN)/tap-server
CLI     := $(BIN)/tap-cli
GUI     := $(BIN)/tap-gui

.PHONY: all deps install build run-server run-client run-client-gui lint fmt test clean

all: build

deps:
	$(GO) mod download
	$(GO) mod tidy

install: deps

build:
	@mkdir -p $(BIN)
	$(GO) build -o $(SERVER) ./cmd/server
	$(GO) build -o $(CLI) ./cmd/cli
	@if [ -d cmd/gui ]; then $(GO) build -o $(GUI) ./cmd/gui; fi

run-server:
	$(GO) run ./cmd/server $(ARGS)

run-client:
	$(GO) run ./cmd/cli -addr $(ADDR) $(ARGS)

run-client-gui:
	@if [ ! -d cmd/gui ]; then echo "GUI client not implemented yet (cmd/gui missing)"; exit 1; fi
	$(GO) run ./cmd/gui -addr $(ADDR) $(ARGS)

lint:
	$(GO) vet ./...
	@unformatted=$$(gofmt -l $$(find . -name '*.go' -not -path './$(BIN)/*')); \
	if [ -n "$$unformatted" ]; then echo "gofmt needed on:"; echo "$$unformatted"; exit 1; fi

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './$(BIN)/*')

test:
	$(GO) test -race ./...

clean:
	rm -rf $(BIN)
	$(GO) clean -cache -testcache
