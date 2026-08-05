BINARY  := gitia
PKG     := github.com/guerrero/gitia
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/internal/cli.Version=$(VERSION) \
	-X $(PKG)/internal/cli.Commit=$(COMMIT) \
	-X $(PKG)/internal/cli.Date=$(DATE)

export CGO_ENABLED := 0

.PHONY: build test lint man install release-dry clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/gitia

test:
	go test ./...

lint:
	golangci-lint run

man:
	@echo "implemented in Task 16"

install:
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/gitia

release-dry:
	@echo "implemented in Task 17"

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist/
