# 0001 — Record architecture decisions

**Status:** Accepted (2026-07-08)

## Context

TypeFerence is a specification with (as of this ADR) two independent implementations.
Decisions about spec semantics, canonicalization, and tooling carry cross-implementation
consequences that are invisible in any single diff. The project needs a durable record of
why things are the way they are, especially where the spec text was ambiguous and a
ruling had to be made.

## Decision

Record architecturally significant decisions as short numbered documents in
`docs/decisions/`, in the format described in that directory's README. A decision is
"architecturally significant" when it changes spec semantics, canonical output bytes,
the trust model, or a dependency boundary — or when two honest readings of the spec
could disagree.

## Consequences

- Spec ambiguities discovered during implementation must produce an ADR and a
  conformance fixture, not a silent ruling buried in code.
- Reviewers can audit the reasoning, not just the result.

## Alternatives considered

- **Commit messages only.** Rejected: not discoverable, not linkable from the spec.
- **A single DECISIONS.md.** Rejected: merge conflicts and no stable per-decision links.
