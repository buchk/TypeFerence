# Determinism guarantees

Identical source must compile to identical bytes — across repeated builds, across
platforms, and across implementations. The guarantee is enforced, not aspirational:

- `conformance/fixtures/` records the expected `typeference-directory-v1` digest of
  every emitted target for 25+ fixtures. Both implementations run the corpus in CI.
- The committed `dist/` tree is the fully materialized reference output for
  `examples/helio`; both implementations byte-compare against it in their own tests.
- The committed root `AGENTS.md` and `dist/maintainer/` are build artifacts of
  `agents/maintainer/`; CI recompiles the definition and fails on any drift.

Rules that protect the guarantee:

- Digest values are regenerated (`go test ./conformance -update`, then verified by
  the C# runner), never typed by hand.
- Canonical serialization is defined in `docs/specification.md` ("Deterministic
  compilation"); any change to it is a specification change with an ADR.
- Nothing about determinism, provenance, or fail-closed behavior is ever relaxed to
  make an unrelated change easier. If a change fights the determinism rules, the
  change is wrong or the specification needs a recorded amendment.
