// Command typeference-lsp is a Language Server Protocol server for TypeFerence
// sources (.tfer and .yaml). It speaks JSON-RPC 2.0 over stdio and publishes
// per-document authoring diagnostics (see internal/lsp for the v0 scope).
package main

import (
	"fmt"
	"os"

	"github.com/buchk/TypeFerence/go/internal/lsp"
)

// version is stamped by the release workflow via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := lsp.NewServer(version).Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "typeference-lsp: %s\n", err)
		os.Exit(1)
	}
}
