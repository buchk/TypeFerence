# Architecture decision records

One short record per direction-level topic: what TypeFerence decided, why, and
what it rejected. They describe the current design, not its history. When a
change moves one of these tradeoffs, the ADR that owns the topic is amended
in place in the same change. Rulings and clarifications belong in the
[specification](../specification.md) with a conformance fixture, not here.

Format: `NNNN-short-title.md` with **Status**, **Context**, **Decision**,
**Consequences**, and **Alternatives considered**.

| ADR | Topic |
| --- | --- |
| [0001](0001-scope-copilot-plugin-authoring.md) | Scope: an authoring and reuse layer for Copilot plugins |
| [0002](0002-one-go-implementation.md) | One Go implementation behind a normative specification |
| [0003](0003-determinism-and-golden-digests.md) | Deterministic output, enforced by golden digests |
| [0004](0004-packages-and-one-marketplace-build.md) | Source packages and one marketplace build |
| [0005](0005-closed-tfer-grammar.md) | A closed `.tfer` grammar with schema-directed scalars |
| [0006](0006-composition-and-templates.md) | Composition: plugins link, profiles embed, skills extend, templates instantiate |
| [0007](0007-build-emits-complete-host-configuration.md) | Build emits complete Copilot configuration |
| [0008](0008-authoring-tools.md) | Authoring tools: import, the setup wizard, and the playground |

The 41 earlier records, which document the path from a general agent-ecosystem
model to this design, were consolidated into these in October 2026. They
remain in git history at
[`3839fb0`](https://github.com/buchk/TypeFerence/tree/3839fb0/docs/decisions).
