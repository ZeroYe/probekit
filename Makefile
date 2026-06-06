VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X probe-agent/internal/selfmetrics.BuildVersion=$(VERSION)"
BIN     := probe-agent

.PHONY: all build clean test lint vet

all: clean test build

# ── Build ──────────────────────────────────────────────────────────────────────

build:
	go build $(LDFLAGS) -o $(BIN)$(exe) ./cmd/probe-agent/

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)-linux-amd64 ./cmd/probe-agent/

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN)-linux-arm64 ./cmd/probe-agent/

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)-windows-amd64.exe ./cmd/probe-agent/

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)-darwin-amd64 ./cmd/probe-agent/

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN)-darwin-arm64 ./cmd/probe-agent/

release: clean build-linux-amd64 build-linux-arm64 build-windows-amd64 build-darwin-amd64 build-darwin-arm64

# ── Test ───────────────────────────────────────────────────────────────────────

test:
	go test -count=1 -timeout 60s ./...

test-race:
	go test -race -count=1 -timeout 120s ./...

test-verbose:
	go test -v -count=1 -timeout 60s ./...

# ── Lint ───────────────────────────────────────────────────────────────────────

lint:
	golangci-lint run ./...

vet:
	go vet ./...

# ── Clean ──────────────────────────────────────────────────────────────────────

clean:
	rm -f $(BIN) $(BIN)-linux-amd64 $(BIN)-linux-arm64 $(BIN)-windows-amd64.exe $(BIN)-darwin-amd64 $(BIN)-darwin-arm64

# ── Run ────────────────────────────────────────────────────────────────────────

run: build
	./$(BIN)$(exe) --config-dir ./config
