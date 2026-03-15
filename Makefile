.PHONY: build test test-race coverage lint fmt clean bench install verify build-librure

BINARY      := ralf
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -s -w -X main.version=$(VERSION)
CGO_ENABLED := 1
LIBRURE_DIR := ./vendor/librure
CGO_LDFLAGS := -L$(LIBRURE_DIR) -lrure -lm -ldl -lpthread
GOFLAGS     := -mod=mod

export CGO_ENABLED CGO_LDFLAGS GOFLAGS

## Build librure from Rust regex-capi
build-librure:
	./scripts/build-librure.sh

## Build
build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/ralf

## Install locally
install:
	go install -ldflags="$(LDFLAGS)" ./cmd/ralf

## Test
test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

## Coverage
coverage:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

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
	rm -f $(BINARY) coverage.out
