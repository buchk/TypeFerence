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
| [0010](0010-browser-playground.md) | Browser playground: the Go compiler as WebAssembly |
| [0011](0011-playground-live-runs.md) | Playground equivalence console: no secrets in the browser |
| [0012](0012-invocation-mode-skill-variants.md) | Invocation-mode skill variants (Proposed) |
| [0013](0013-user-defined-typed-context.md) | User-defined typed context (Proposed) |
| [0014](0014-go-only-implementation.md) | Go-only implementation; the spec is the open invitation (Accepted) |
| [0015](0015-exposure-and-visibility.md) | Exposure and visibility (Proposed) |
| [0016](0016-sealing-mutability-presence.md) | Sealing: mutability and presence (Proposed) |
| [0017](0017-tools-as-extern.md) | Tools as extern declarations (Proposed) |
| [0018](0018-callable-resource-card-and-publishing.md) | Callable-resource card and publishing (Proposed) |
| [0019](0019-context-lifecycles-two-doors.md) | Context lifecycles: the two doors (Proposed) |
