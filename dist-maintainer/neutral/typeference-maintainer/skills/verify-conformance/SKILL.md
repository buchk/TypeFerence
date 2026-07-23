---
name: verify-conformance
description: "Runs the determinism suite and reports whether the compiler reproduces the committed digests."
---

Run `go test ./conformance` from the go/ directory (or `make conformance`).
Report passed=true only when every fixture reproduces its committed digest.
List each failing fixture and target as a mismatch. Never resolve a mismatch
by editing a digest; find the compiler regression, or take the ruling to the
specification with an ADR and a regenerated fixture.

## Context loaded on invocation

- `context/determinism.md`
