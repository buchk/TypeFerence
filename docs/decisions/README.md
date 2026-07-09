# Architecture decision records

Numbered, immutable records of decisions that shaped TypeFerence. Each ADR captures one
decision, the alternatives considered, and the consequences. ADRs are never edited to
change a decision; a later ADR supersedes an earlier one and says so.

Format: `NNNN-short-title.md` with sections **Status**, **Context**, **Decision**,
**Consequences**, **Alternatives considered**.

| ADR | Title |
| --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions |
| [0002](0002-cli-verb-typeference.md) | The CLI verb is `typeference` |
| [0003](0003-go-implementation-layout.md) | Go implementation layout and dependency policy |
| [0004](0004-canonicalization-rulings.md) | Canonicalization rulings for cross-implementation byte identity |
| [0005](0005-conformance-suite.md) | Cross-implementation conformance suite design |
| [0006](0006-self-hosting-design-feedback.md) | Self-hosting the maintainer agent, and what it revealed |
| [0007](0007-release-distribution.md) | Release distribution: static binaries, no installer |
| [0008](0008-eval-harness-scope.md) | Behavioral eval harness: scope and design |
| [0009](0009-behavioral-equivalence-harness.md) | BETH: the behavioral equivalence test harness |
