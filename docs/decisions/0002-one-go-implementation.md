# 0002 — One Go implementation behind a normative specification

## Status

Accepted (2026-10-07).

## Context

TypeFerence once had a C# reference implementation and a Go implementation
checked against each other. Two implementations proved the specification was
implementable, but every change was built twice, and the people who use the
tool want a binary, not a standard.

## Decision

1. Go is the only implementation. The CLI is `typeference`, built from
   `go/cmd/typeference` as a static binary per platform with no runtime
   dependencies; releases follow `docs/release-checklist.md`.
2. `docs/specification.md` stays normative. Where the specification and the
   implementation disagree, the specification wins, and ambiguity is a
   specification bug. Semantic changes amend the specification first.
3. The same packages build for WebAssembly (`go/cmd/typeference-wasm`) so the
   playground and setup wizard run the real compiler (ADR-0008). The language
   server (`go/cmd/typeference-lsp`) uses the same loader and compiler.
4. The specification is enforced by golden fixtures rather than by a second
   implementation (ADR-0003).

## Consequences

- Every change is implemented once.
- "Normative, proven by two implementations" became "normative, realized by
  one". A second implementation could return; the fixture corpus is
  language-neutral.

## Alternatives considered

- **Keep both implementations.** Worth it only for a standard others
  implement; for an installed tool it taxes the wrong axis.
- **Drop the specification and let the code define the language.** Fastest,
  but behaviour would become whatever the code happens to do, and review would
  lose its reference.
