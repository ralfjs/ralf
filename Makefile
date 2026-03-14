.PHONY: build test test-race lint fmt clean bench install verify

BINARY     := bepro
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -s -w -X main.version=$(VERSION)
CGO_ENABLED := 1

## Build
build:
	CGO_ENABLED=$(CGO_ENABLED) go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/bepro

## Install locally
install:
	CGO_ENABLED=$(CGO_ENABLED) go install -ldflags="$(LDFLAGS)" ./cmd/bepro

## Test
test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

## Benchmark
bench:
	go test -bench=. -benchmem ./internal/engine/

## Lint
lint:
	golangci-lint run ./...

## Format
fmt:
	gofumpt -w .

## Verify (CI: format check + mod tidy check)
verify: lint
	gofumpt -d . | (! grep .)
	go mod tidy
	git diff --exit-code go.mod go.sum

## Clean
clean:
	rm -f $(BINARY)
