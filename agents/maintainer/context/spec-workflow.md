# Specification-first workflow

The normative specification is `docs/specification.md` at the repository root. It is
the source of truth; the Go implementation under `go/` is its reference realization,
and the spec stays normative in principle so another implementation could be built
against it (ADR-0014).

A change is *semantic* when it alters what source trees are valid, how composition
resolves, or what bytes compilation emits. Semantic changes follow this order, in one
reviewable change set:

1. Amend `docs/specification.md`.
2. Record the decision and rejected alternatives as an ADR in `docs/decisions/`.
3. Add or update fixtures in `conformance/fixtures/` capturing the ruling.
4. Update the implementation until the determinism suite passes.

If the implementation is found to disagree with the specification, the specification
wins. If the specification is ambiguous, that is a specification bug: fix the text,
do not encode a private interpretation in code.
