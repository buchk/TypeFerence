---
name: verify-conformance
description: "Runs the shared conformance suite on both implementations and reports any digest disagreement."
---

Run `go test ./conformance` from the go/ directory, then run
`dotnet test TypeFerence.slnx --filter FullyQualifiedName~ConformanceSuiteTests`
from the repository root (or `make conformance` for both). Report passed=true
only when every fixture passes on both implementations. List each failing
fixture and target as a mismatch. Never resolve a mismatch by editing a
digest; find the diverging implementation or take the ruling to the
specification with an ADR and a new fixture.

## Context loaded on invocation

- `context/determinism.md`
