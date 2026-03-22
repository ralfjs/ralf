.PHONY: build test test-race coverage lint fmt clean bench install verify build-librure

BINARY      := ralf
LDFLAGS     := -s -w
CGO_ENABLED := 1
LIBRURE_DIR := ./vendor/librure
UNAME_S     := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
CGO_LDFLAGS := -L$(LIBRURE_DIR) -lrure -lm -lpthread
else
CGO_LDFLAGS := -L$(LIBRURE_DIR) -lrure -lm -ldl -lpthread
endif
GOFLAGS     := -mod=mod

export CGO_ENABLED CGO_LDFLAGS GOFLAGS

## Build librure from Rust regex-capi
build-librure:
	./scripts/build-librure.sh

## Build
build:
	@mkdir -p build
	go build -ldflags="$(LDFLAGS)" -o build/$(BINARY) ./cmd/ralf

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
	rm -rf build/ coverage.out
