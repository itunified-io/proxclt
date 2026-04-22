BINARY := proxctl
PKG    := github.com/itunified-io/proxctl
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(PKG)/pkg/version.Version=$(VERSION) \
	-X $(PKG)/pkg/version.Commit=$(COMMIT) \
	-X $(PKG)/pkg/version.Date=$(DATE)

.PHONY: build test lint vet staticcheck docs docs-cli clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/proxctl

test:
	go test ./...

vet:
	go vet ./...

staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || { echo "staticcheck not installed — go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	staticcheck ./...

lint: vet staticcheck

docs: docs-cli

docs-cli:
	@mkdir -p docs/cli-reference
	@rm -f docs/cli-reference/proxctl*.md
	go run ./cmd/docgen docs/cli-reference
	@echo "→ docs/cli-reference/ (cobra/doc.GenMarkdownTree)"

clean:
	rm -rf bin/ dist/
