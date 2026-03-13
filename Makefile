# Codex Library Server — Makefile
# All Go commands include -tags fts5 to enable SQLite FTS5 support.

GO_TAGS := -tags fts5
BINARY  := codex

.PHONY: build test vet lint run clean

## Build the server binary
build:
	go build $(GO_TAGS) -o $(BINARY) ./cmd/codex

## Run all tests
test:
	go test $(GO_TAGS) ./...

## Run tests with race detector
test-race:
	go test $(GO_TAGS) -race ./...

## Run go vet
vet:
	go vet $(GO_TAGS) ./...

## Run staticcheck (if installed)
lint:
	staticcheck $(GO_TAGS) ./...

## Run the server (development)
run:
	go run $(GO_TAGS) ./cmd/codex --config config.yaml

## Remove build artifacts
clean:
	rm -f $(BINARY)
