# Specification-first workflow

The normative specification is `docs/specification.md` at the repository root. It is
the source of truth for both implementations:

- C# reference implementation: `src/TypeFerence.Core`, `src/TypeFerence.Cli`
- Go implementation: `go/`

A change is *semantic* when it alters what source trees are valid, how composition
resolves, or what bytes compilation emits. Semantic changes follow this order, in one
reviewable change set:

1. Amend `docs/specification.md`.
2. Record the decision and rejected alternatives as an ADR in `docs/decisions/`.
3. Add or update fixtures in `conformance/fixtures/` capturing the ruling.
4. Update both implementations until the conformance suite passes on both.

If an implementation is found to disagree with the specification, the specification
wins. If the specification is ambiguous enough that two honest implementations could
diverge, that is a specification bug: fix the text, do not encode a private
interpretation in code.
