# TypeFerence build entry points. Requires: Go 1.24+, .NET 10 SDK (reference
# implementation only). All artifacts are deterministic; run `make conformance`
# to verify both implementations agree byte-for-byte.

GO ?= go
DOTNET ?= dotnet
GOFLAGS := -trimpath
LDFLAGS := -s -w
BINDIR := bin

.PHONY: all build build-go build-dotnet test test-go test-dotnet conformance \
	fmt vet clean release-binaries

all: build test

build: build-go build-dotnet

# Single static binary, no runtime dependencies.
build-go:
	cd go && CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o ../$(BINDIR)/typeference$(shell $(GO) env GOEXE) ./cmd/typeference

build-dotnet:
	$(DOTNET) build TypeFerence.slnx

test: test-go test-dotnet

test-go:
	cd go && $(GO) test ./...

test-dotnet:
	$(DOTNET) test TypeFerence.slnx

# Cross-implementation conformance: both compilers run the shared fixture
# corpus; digests must match the manifests and each other.
conformance:
	cd go && $(GO) test ./conformance -run TestConformance -v
	$(DOTNET) test TypeFerence.slnx --filter FullyQualifiedName~ConformanceSuiteTests

fmt:
	cd go && gofmt -l -w .
	$(DOTNET) format TypeFerence.slnx

vet:
	cd go && $(GO) vet ./...
	cd go && test -z "$$(gofmt -l .)"

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
	cd $(BINDIR) && sha256sum typeference-* > SHA256SUMS
