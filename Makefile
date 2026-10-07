# TypeFerence build entry points. Requires: Go 1.24+ and nothing else. All
# artifacts are deterministic; run `make conformance` to verify the compiler
# reproduces the committed digests byte-for-byte (ADR-0003).

GO ?= go
VERSION ?= dev
GOFLAGS := -trimpath
LDFLAGS := -s -w -X main.version=$(VERSION)
BINDIR := bin

.PHONY: all build build-go build-lsp test test-go conformance \
	selfhost selfhost-check fmt vet clean release-binaries playground

all: build test

build: build-go build-lsp

# Single static binary, no runtime dependencies.
build-go:
	cd go && CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference$(shell $(GO) env GOEXE) ./cmd/typeference

# Language server for version 6 .tfer package authoring (internal/lsp).
build-lsp:
	cd go && CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-lsp$(shell $(GO) env GOEXE) ./cmd/typeference-lsp

test: test-go

test-go:
	cd go && $(GO) test ./...

# Determinism suite: the compiler compiles the shared fixture corpus and must
# reproduce the committed digests (ADR-0003).
conformance:
	cd go && $(GO) test ./conformance -v

# Recompile the self-hosted maintainer definition (agents/maintainer) into its
# committed artifacts: dist-maintainer (the installable Copilot plugin) and the
# repository-root AGENTS.md, which is the maintainer agent file without its
# frontmatter.
MAINTAINER_AGENT := dist-maintainer/agent-plugin/typeference-maintainer/com.github.copilot/agents/typeference-maintainer.agent.md
STRIP_FRONTMATTER := awk 'n >= 2 && (seen || $$0 != "") { seen = 1; print } /^---$$/ { n++ }'

selfhost: build-go
	$(BINDIR)/typeference$(shell $(GO) env GOEXE) build agents/maintainer --out dist-maintainer
	$(STRIP_FRONTMATTER) $(MAINTAINER_AGENT) > AGENTS.md

# Fail if the committed artifacts have drifted from the definition.
selfhost-check: build-go
	$(BINDIR)/typeference$(shell $(GO) env GOEXE) diff agents/maintainer --against dist-maintainer
	$(STRIP_FRONTMATTER) $(MAINTAINER_AGENT) | cmp - AGENTS.md

# Regenerate the committed reference output of the Helio marketplace
# (examples/helio): the reference test stages its packages, restores the
# marketplace, builds it, and rewrites dist/.
reference:
	cd go && $(GO) test ./internal/compile -run TestHelioReference -update

fmt:
	cd go && gofmt -l -w .

vet:
	cd go && $(GO) vet ./...
	cd go && test -z "$$(gofmt -l .)"

# Browser playground: the unmodified Go compiler built for js/wasm plus a
# static, dependency-free UI. Everything lands in web/playground; serve that
# directory with any static file server. wasm_exec.js is copied from the
# building toolchain so it always matches the wasm binary's Go version.
playground:
	cd go && GOOS=js GOARCH=wasm $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../web/playground/typeference.wasm ./cmd/typeference-wasm
	cp "$$($(GO) env GOROOT)/lib/wasm/wasm_exec.js" web/playground/wasm_exec.js
	cd go && $(GO) run ./cmd/playground-pack -root .. -out ../web/playground/examples.json

clean:
	rm -rf $(BINDIR)

# Local prebuilt binaries for the supported platforms. Not published; see
# docs/release-checklist.md.
release-binaries:
	cd go && CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-linux-amd64 ./cmd/typeference
	cd go && CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-linux-arm64 ./cmd/typeference
	cd go && CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-darwin-amd64 ./cmd/typeference
	cd go && CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-darwin-arm64 ./cmd/typeference
	cd go && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-windows-amd64.exe ./cmd/typeference
	cd go && CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference-windows-arm64.exe ./cmd/typeference
	cd $(BINDIR) && sha256sum typeference-* > SHA256SUMS
