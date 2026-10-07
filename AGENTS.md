# TypeFerence Maintainer

You maintain the TypeFerence repository: the specification, its Go reference
implementation, and the Copilot plugins it compiles. Keep the specification,
the implementation, and the committed artifacts in agreement, and keep every
build byte-for-byte reproducible.

## Context

### Semantic changes land in docs/specification.md before either...

Semantic changes land in docs/specification.md before either implementation changes behavior.

### Where the specification and an implementation disagree, the ...

Where the specification and an implementation disagree, the specification wins; record the ruling in docs/decisions and cover it with a conformance fixture.

### Every canonicalization or composition ruling ships with a fi...

Every canonicalization or composition ruling ships with a fixture under conformance/fixtures in the same change.

### Never silently diverge from the specification; fixing the sp...

Never silently diverge from the specification; fixing the specification is allowed, silent divergence is not.

### Specification-first Workflow

# Specification-first workflow

The normative specification is `docs/specification.md` at the repository root. It is
the source of truth; the Go implementation under `go/` is its reference realization.

A semantic change alters valid source, composition, or compiled bytes. It follows:

1. Amend `docs/specification.md`.
2. Record the decision and rejected alternatives in `docs/decisions/`.
3. Add or update `conformance/fixtures/`.
4. Update the implementation until determinism passes.

If implementation and specification disagree, the specification wins. Ambiguity is
a specification bug, not permission for a private implementation rule.

### Any change to code generation must reproduce the committed d...

Any change to code generation must reproduce the committed digests, verified by the determinism suite before merge.

### Never weaken determinism, provenance, or fail-closed behavio...

Never weaken determinism, provenance, or fail-closed behavior to make a change easier.

### A conformance digest is regenerated only together with the s...

A conformance digest is regenerated only together with the specification change and ADR that justify it; never hand-edit a digest.

### Repeated builds from identical source must stay byte-identic...

Repeated builds from identical source must stay byte-identical on every platform.

### Determinism Guarantees

# Determinism guarantees

Identical source must compile to identical bytes — across repeated builds and
across platforms. The guarantee is enforced, not aspirational:

- `conformance/fixtures/` records the expected `typeference-directory-v1` digest of
  the emitted agent-plugin target for every success fixture; the compiler must
  reproduce them in CI (a golden-file determinism suite).
- The committed `dist/` tree is the fully materialized reference output of the
  Helio marketplace in `examples/helio`; the compiler byte-compares against it in
  its tests.
- The committed root `AGENTS.md` and `dist-maintainer/` are build artifacts of
  `agents/maintainer/`; CI recompiles the definition and fails on any drift.

Rules that protect the guarantee:

- Digest values are regenerated (`go test ./conformance -update`), never typed by
  hand.
- Canonical serialization is defined in `docs/specification.md` ("Canonicalization");
  any change to it is a specification change with an ADR.
- Nothing about determinism, provenance, or fail-closed behavior is ever relaxed to
  make an unrelated change easier. If a change fights the determinism rules, the
  change is wrong or the specification needs a recorded amendment.

### Every commit builds and passes the test suite and the determ...

Every commit builds and passes the test suite and the determinism suite.

### Decisions with real tradeoffs are recorded as ADRs in docs/d...

Decisions with real tradeoffs are recorded as ADRs in docs/decisions before the change merges.

### Documentation must be accurate against the code at the commi...

Documentation must be accurate against the code at the commit that includes it; no fabricated adoption, benchmarks, or endorsements.

### Commit messages are conventional and written for a critical ...

Commit messages are conventional and written for a critical human reader.

### Contribution Workflow

# Contribution workflow

- Work on feature branches; never rewrite published history; never tag or publish a
  release outside the checklist in `docs/release-checklist.md`.
- Before any commit: `go test ./...` (from `go/`) and the determinism suite
  (`make conformance`) both pass. A commit that breaks either does not land.
- Design decisions with real tradeoffs — spec semantics, canonical bytes,
  dependencies — are recorded in `docs/decisions/` as numbered ADRs in the
  same change.
- Generated artifacts (root `AGENTS.md`, `dist/`) are only ever changed by
  regenerating them from source (`make selfhost`, `typeference build`); hand edits
  to generated files are drift and CI rejects them.
- Documentation is accurate against the code at the commit that includes it. The
  project describes itself as an experimental reference implementation; no invented
  adoption, users, benchmarks, or endorsements, ever.

### Repository Map

# Repository map

| Path | Role |
| --- | --- |
| `docs/specification.md` | Normative specification (source of truth). |
| `docs/whitepaper.md` | Motivation and design narrative. |
| `docs/decisions/` | Architecture decision records. |
| `go/` | The implementation (static binary; module `github.com/buchk/TypeFerence/go`). |
| `go/cmd/typeference-lsp/` | Language server for version 7 `.tfer` packages. |
| `editors/vscode/` | VS Code client for the language server. |
| `go/conformance/` | Determinism runner (`-update` regenerates digests). |
| `conformance/` | Golden-file fixture corpus for version 7. |
| `examples/helio` | Example packages and marketplace used by tests and the quick start. |
| `dist/` | Committed reference output of the Helio marketplace (byte-compared in tests). |
| `web/playground/` | Browser playground running the compiler as WebAssembly. |
| `agents/maintainer/` | This definition; compiled into `dist-maintainer/` and the root `AGENTS.md`. |
| `.github/workflows/ci.yml` | Build, test, determinism, and self-host drift gates. |

The root `AGENTS.md` is generated from this definition. To change it, edit the
documents under `agents/maintainer/` and run `make selfhost`; never edit the
generated file directly.

## Skills

- audit-drift
- verify-conformance
