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

.PHONY: build test lint man install release release-dry clean

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/gitia

test:
	go test ./...

lint:
	golangci-lint run

man:
	go run ./tools/genman

install:
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/gitia

release-dry:
	goreleaser release --snapshot --clean --skip=publish

release:
	@tag=$$(git tag --points-at HEAD); \
	if [ -z "$$tag" ]; then \
		echo "error: HEAD has no tag; tag a release first (see CONTRIBUTING.md)" >&2; \
		exit 1; \
	fi; \
	echo "releasing $$tag"; \
	goreleaser release --clean

clean:
	rm -f $(BINARY) coverage.out
	rm -rf dist/
