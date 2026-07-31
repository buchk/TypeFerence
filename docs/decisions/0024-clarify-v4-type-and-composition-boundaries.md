# 0024 — Clarify version 4 type and composition boundaries

**Status:** Accepted (2026-07-30)

## Context

The first object-model ADRs used several analogies—structural interfaces,
context-type refinement, skill/tool siblings, target-selected variants, and
sealed members—before all of their interactions were implemented. Version 4 now
needs one coherent account of identity, satisfaction, publication, runtime
imports, and equal-path composition.

The unresolved wording creates concrete contradictions:

- structurally identical context types could appear to cross a governance
  boundary without declaring the refinement;
- tools are described both as independent runtime dependencies and as
  implementations of capabilities, although they do not bind capabilities;
- target-driven build selection conflicts with the later build/link split; and
- the specification permits an identical promoted capability diamond while the
  resolver rejects it.

## Decision

### Structural behavior and nominal context trust

Interfaces are satisfied structurally from the resolved slots and capability
bindings of an agent. Capability identity remains its exact resource ID.

Context-type satisfaction is nominal-by-declared-refinement: a context satisfies
its declared `contextType` and the transitive set of context types explicitly
named through `embeds`. The member shape of each declared refinement is checked
structurally. A separately declared lookalike type does not satisfy a governed
type merely because its fields match.

Inherited context fields must agree on type, requiredness, and default semantics.
An identical inherited declaration may be deduplicated. Conflicting defaults or
modifier state from sibling bases are ambiguous unless the derived context type
redeclares the field locally with one compatible resolution.

### Visibility is orthogonal to satisfaction

`visibility` belongs to a capability and is `internal` by default or `exposed`.
Both internal and exposed capabilities participate in composition and structural
interface satisfaction. Visibility controls only the public callable projection:
only exposed capabilities appear in A2A/ARD callable surfaces. Exposure follows
the capability through embedding.

### Tools are independent extern dependencies

A capability is an agent behavior contract and a skill is its model-implemented
fulfillment. A tool is not the code-implemented sibling of that skill. It is an
independent, versioned extern contract required by a skill or variant and fulfilled
by deployment.

The exact tool ID names the required input/output contract. A deployment binding
attests that a provider's remote name implements that contract. TypeFerence
validates the declaration, the binding's presence, provider shape, selected mode,
and host-native serialization; it does not interrogate the remote provider or
prove its runtime schema or authorization behavior. Provider credentials remain
deployment references, not source type refinements.

### Build preserves modes; link records deployment selection

Build preserves every neutral rendering and may materialize one fixed mode for a
target adapter. Codex, Copilot, and Cursor materialize `manual` for a multimodal
skill. There is no general build-time mode override.

Link validates the deployment's selected modes against the already-built
renderings and records the canonical selected-mode set even when no tool binding
exists. It may project a selected rendering into a future host adapter but may
not synthesize or alter instructions. Fixed-mode adapters require exactly their
built mode.

This supersedes ADR 0012 section 5.

### Identical promoted capability diamonds converge

At the same winning depth, promoted capability bindings with the same resolved
implementation and the same modifier state are one semantic member and are
deduplicated. Different implementations or incompatible modifier state remain
ambiguous unless resolved locally. Provenance retains every contributing path.
Sealing is checked after identical candidates converge, so a sealed member reached
through two paths is not rejected merely for forming a diamond.

## Consequences

- Behavioral interfaces remain convenient and structural while context-governance
  boundaries require an explicit nominal relationship.
- Internal implementation behavior can satisfy contracts without accidentally
  becoming public API.
- Tool bindings are honest deployment attestations rather than unverifiable
  compiler claims.
- Mode selection is observable in every linked artifact, including tool-free ones.
- Common identical embedding diamonds compose; genuine implementation conflicts
  still fail closed.
- This ADR supersedes ADR 0012 section 5, ADR 0013 decision 2's structural-
  satisfaction wording, ADR 0016's temporary sealed-diamond limitation, and ADR
  0017 decision 2's skill/tool-sibling model.

## Alternatives considered

- **Purely structural context satisfaction.** Lets an unrelated lookalike type
  impersonate a governed refinement; rejected.
- **Make every tool bind a capability.** Conflates an agent's offered behavior with
  the lower-level runtime operations its skills consume; deferred as a possible
  future fulfillment kind rather than imposed on all externs.
- **Select variants during build with a free-form flag.** Makes one source produce
  hidden instruction differences outside the target adapter identity; rejected.
- **Reject every equal-depth diamond.** Predictable but unnecessarily hostile to
  normal composition and inconsistent with the specification's distinction
  between identical and different implementations; rejected.
