# Determinism guarantees

Identical source must compile to identical bytes — across repeated builds and
across platforms. The guarantee is enforced, not aspirational:

- `conformance/fixtures/` records the expected `typeference-directory-v1` digest of
  every emitted target for the fixture corpus; the compiler must reproduce them in
  CI (a golden-file determinism suite).
- The committed `dist/` tree is the fully materialized reference output for
  `examples/helio`; the compiler byte-compares against it in its tests.
- The committed root `AGENTS.md` and `dist-maintainer/` are build artifacts of
  `agents/maintainer/`; CI recompiles the definition and fails on any drift.

Rules that protect the guarantee:

- Digest values are regenerated (`go test ./conformance -update`), never typed by
  hand.
- Canonical serialization is defined in `docs/specification.md` ("Deterministic
  compilation"); any change to it is a specification change with an ADR.
- Nothing about determinism, provenance, or fail-closed behavior is ever relaxed to
  make an unrelated change easier. If a change fights the determinism rules, the
  change is wrong or the specification needs a recorded amendment.
