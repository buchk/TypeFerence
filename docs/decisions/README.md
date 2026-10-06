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
| [0008](0008-eval-harness-scope.md) | Behavioral eval harness: scope and design | Accepted | Evaluation commands removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0009](0009-behavioral-equivalence-harness.md) | BETH: the behavioral equivalence test harness | Accepted | Removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0010](0010-browser-playground.md) | Browser playground: the Go compiler as WebAssembly | Accepted | — |
| [0011](0011-playground-live-runs.md) | Playground equivalence console: no secrets in the browser | Accepted | Removed with the equivalence console by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0012](0012-invocation-mode-skill-variants.md) | Invocation-mode skill variants | Accepted | Format and mode timing partly superseded by [0023](0023-tfer-source-format.md) and [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0013](0013-user-defined-typed-context.md) | User-defined typed context | Accepted | Compatibility, format, and satisfaction partly superseded by [0020](0020-close-the-source-language.md), [0023](0023-tfer-source-format.md), and [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0014](0014-go-only-implementation.md) | Go-only implementation; the spec is the open invitation | Accepted | — |
| [0015](0015-exposure-and-visibility.md) | Exposure and visibility | Accepted | Go export analogy narrowed by [0024](0024-clarify-v4-type-and-composition-boundaries.md); visibility removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0016](0016-sealing-mutability-presence.md) | Sealing: mutability and presence | Accepted | Deferred compatibility closed by [0020](0020-close-the-source-language.md); diamond behavior clarified by [0024](0024-clarify-v4-type-and-composition-boundaries.md); skill-level sealing proposed in [0031](0031-additive-skill-extension.md); sealing removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0017](0017-tools-as-extern.md) | Tools as extern declarations | Accepted | Deferred representations and sibling model partly superseded by [0020](0020-close-the-source-language.md) and [0024](0024-clarify-v4-type-and-composition-boundaries.md); replaced by servers in [0037](0037-v7-build-emitted-host-configuration.md) |
| [0018](0018-callable-resource-card-and-publishing.md) | Callable-resource card and publishing | Accepted | Runtime boundary completed by [0022](0022-build-link-and-source-identity.md); callable publication removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0019](0019-context-lifecycles-two-doors.md) | Context lifecycles: the two doors | Accepted | Raw-path compatibility closed by [0020](0020-close-the-source-language.md); door A narrowed to documents and typed data by [0036](0036-v7-documents-data-and-templates.md) |
| [0020](0020-close-the-source-language.md) | Close the source language and preserve modes | Accepted | Mode-selection timing clarified by [0024](0024-clarify-v4-type-and-composition-boundaries.md) |
| [0021](0021-restore-locked-source-packages.md) | Restore exact, locked source packages | Accepted | Extended to organization marketplaces by [0034](0034-organization-marketplace-from-source-packages.md) |
| [0022](0022-build-link-and-source-identity.md) | Separate build, link, and source identity | Accepted | Link removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) and [0037](0037-v7-build-emitted-host-configuration.md) |
| [0023](0023-tfer-source-format.md) | Define the `.tfer` source format | Accepted | Dual-format acceptance superseded by [0026](0026-v5-closed-frontmatter-grammar.md) |
| [0024](0024-clarify-v4-type-and-composition-boundaries.md) | Clarify version 4 type and composition boundaries | Accepted | — |
| [0025](0025-own-linked-output-before-reset.md) | Require linked-output ownership before reset | Accepted | Link removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0026](0026-v5-closed-frontmatter-grammar.md) | v5: one closed frontmatter grammar and syntactic scalar typing | Accepted | Parser and scalar-typing rulings proposed for amendment by [0032](0032-v6-grammar-and-schema-directed-scalars.md) |
| [0027](0027-v5-no-untyped-behavioral-prose.md) | v5: no untyped behavioral prose | Accepted | Decision 3 proposed for supersession by [0029](0029-agent-plugins-primary-target.md); superseded by the provenance rule in [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0028](0028-deterministic-setup-wizard.md) | Deterministic setup wizard: one generator, browser and CLI | Accepted | Generator output moved to version 6 by [0030](0030-plugin-documents-and-v6-authoring.md) |
| [0029](0029-agent-plugins-primary-target.md) | GitHub Agent Plugins as the primary output target | Proposed | Amended by [0035](0035-v7-copilot-plugin-authoring-layer.md) and [0037](0037-v7-build-emitted-host-configuration.md) |
| [0030](0030-plugin-documents-and-v6-authoring.md) | Plugin documents, thin agents, and v6 authoring | Proposed | Independence and slots replaced by [0036](0036-v7-documents-data-and-templates.md) |
| [0031](0031-additive-skill-extension.md) | Additive skill extension | Proposed | Skill sealing removed by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0032](0032-v6-grammar-and-schema-directed-scalars.md) | v6: the closed grammar as the only parser, and schema-directed scalars | Proposed | Folded into version 7 by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0033](0033-import-copilot-customizations.md) | Import existing GitHub Copilot customizations | Proposed | Extended to skill files and servers by [0036](0036-v7-documents-data-and-templates.md) and [0037](0037-v7-build-emitted-host-configuration.md) |
| [0034](0034-organization-marketplace-from-source-packages.md) | One organization marketplace, built from source packages | Proposed | Folded into version 7 by [0035](0035-v7-copilot-plugin-authoring-layer.md) |
| [0035](0035-v7-copilot-plugin-authoring-layer.md) | Version 7: an authoring and reuse layer for Copilot plugins | Proposed | — |
| [0036](0036-v7-documents-data-and-templates.md) | Version 7: documents, typed data, and templates | Proposed | — |
| [0037](0037-v7-build-emitted-host-configuration.md) | Version 7: MCP servers and Copilot fields emitted by build | Proposed | — |
| [0038](0038-v7-review-rulings.md) | Version 7 review rulings: portable bytes, requirements, provenance, and inherited bindings | Proposed | — |
