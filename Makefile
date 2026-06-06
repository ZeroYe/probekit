VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X github.com/ZeroYe/probekit/internal/selfmetrics.BuildVersion=$(VERSION)"
BIN     := ProbeKit

.PHONY: all build clean test lint vet

all: clean test build

# ── Build ──────────────────────────────────────────────────────────────────────

build:
	go build $(LDFLAGS) -o $(BIN)$(exe) ./cmd/ProbeKit/

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)-linux-amd64 ./cmd/ProbeKit/

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BIN)-linux-arm64 ./cmd/ProbeKit/

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BIN)-windows-amd64.exe ./cmd/ProbeKit/

release: clean build-linux-amd64 build-linux-arm64 build-windows-amd64

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
	rm -f $(BIN) $(BIN)-linux-amd64 $(BIN)-linux-arm64 $(BIN)-windows-amd64.exe

# ── Run ────────────────────────────────────────────────────────────────────────

run: build
	./$(BIN)$(exe) --config-dir ./config
