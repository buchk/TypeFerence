# Architecture decision records

Numbered, immutable records of decisions that shaped TypeFerence. Each ADR captures one
decision, the alternatives considered, and the consequences. ADRs are never edited to
change a decision; a later ADR supersedes an earlier one and says so.

Format: `NNNN-short-title.md` with sections **Status**, **Context**, **Decision**,
**Consequences**, **Alternatives considered**. Accepted records remain historical;
the relationship column identifies later records that replace or narrow part of
their decision.

| ADR | Title | Status | Later relationship |
| --- | --- | --- | --- |
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted | — |
| [0002](0002-cli-verb-typeference.md) | The CLI verb is `typeference` | Accepted | — |
| [0003](0003-go-implementation-layout.md) | Go implementation layout and dependency policy | Accepted | Dual-implementation premise superseded by [0014](0014-go-only-implementation.md) |
| [0004](0004-canonicalization-rulings.md) | Canonicalization rulings for cross-implementation byte identity | Accepted | Cross-implementation framing superseded by [0014](0014-go-only-implementation.md); byte rulings remain normative |
| [0005](0005-conformance-suite.md) | Cross-implementation conformance suite design | Accepted | Runner model superseded by [0014](0014-go-only-implementation.md); golden-corpus design remains |
| [0006](0006-self-hosting-design-feedback.md) | Self-hosting the maintainer agent, and what it revealed | Accepted | — |
| [0007](0007-release-distribution.md) | Release distribution: static binaries, no installer | Accepted | Implementation inventory updated by [0014](0014-go-only-implementation.md) |
| [0008](0008-eval-harness-scope.md) | Behavioral eval harness: scope and design | Accepted | — |
| [0009](0009-behavioral-equivalence-harness.md) | BETH: the behavioral equivalence test harness | Accepted | — |
| [0010](0010-browser-playground.md) | Browser playground: the Go compiler as WebAssembly | Accepted | — |
| [0011](0011-playground-live-runs.md) | Playground equivalence console: no secrets in the browser | Accepted | — |
| [0012](0012-invocation-mode-skill-variants.md) | Invocation-mode skill variants | Accepted | Format and mode timing partly superseded by [0023](0023-tfer-source-format.md) and [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0013](0013-user-defined-typed-context.md) | User-defined typed context | Accepted | Compatibility, format, and satisfaction partly superseded by [0020](0020-close-the-source-language.md), [0023](0023-tfer-source-format.md), and [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0014](0014-go-only-implementation.md) | Go-only implementation; the spec is the open invitation | Accepted | — |
| [0015](0015-exposure-and-visibility.md) | Exposure and visibility | Accepted | Go export analogy narrowed by [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0016](0016-sealing-mutability-presence.md) | Sealing: mutability and presence | Accepted | Deferred compatibility closed by [0020](0020-close-the-source-language.md); diamond behavior clarified by [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0017](0017-tools-as-extern.md) | Tools as extern declarations | Accepted | Deferred representations and sibling model partly superseded by [0020](0020-close-the-source-language.md) and [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0018](0018-callable-resource-card-and-publishing.md) | Callable-resource card and publishing | Accepted | Runtime boundary completed by [0022](0022-build-link-and-source-identity.md) |
| [0019](0019-context-lifecycles-two-doors.md) | Context lifecycles: the two doors | Accepted | Raw-path compatibility closed by [0020](0020-close-the-source-language.md) |
| [0020](0020-close-the-source-language.md) | Close the source language and preserve modes | Accepted | Mode-selection timing clarified by [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0021](0021-restore-locked-source-packages.md) | Restore exact, locked source packages | Accepted | — |
| [0022](0022-build-link-and-source-identity.md) | Separate build, link, and source identity | Accepted | — |
| [0023](0023-tfer-source-format.md) | Define the `.tfer` source format | Accepted | Dual-format acceptance superseded by [0026](0026-v5-closed-frontmatter-grammar.md) |
| [0024](0024-clarify-v4-type-and-composition-boundaries.md) | Clarify version 4 type and composition boundaries | Accepted | — |
| [0025](0025-own-linked-output-before-reset.md) | Require linked-output ownership before reset | Accepted | — |
| [0026](0026-v5-closed-frontmatter-grammar.md) | v5: one closed frontmatter grammar and syntactic scalar typing | Accepted | — |
| [0027](0027-v5-no-untyped-behavioral-prose.md) | v5: no untyped behavioral prose | Accepted | — |
| [0028](0028-deterministic-setup-wizard.md) | Deterministic setup wizard: one generator, browser and CLI | Accepted | — |
