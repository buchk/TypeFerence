# 0012 — Invocation-mode skill variants

**Status:** Proposed (2026-07-22)

## Context

A capability is a fixed contract: a versioned method slot with a stable
`inputSchema`/`outputSchema`. A skill is a concrete implementation that binds one
capability.

Real agents need the *same* capability delivered differently to different
consumers. The motivating case is a repository-status agent whose output is
consumed three ways:

- **pipeline** — automated/CI invocation; strict machine output, no prose, never
  asks a clarifying question, marks unavailable signals explicitly.
- **manual** — a human in an interactive host (Copilot, Cursor); explains
  reasoning, surfaces risk conversationally, offers the next accountable action.
- **a2a** — invoked by another agent (the cross-agent request path Helio already
  exercises when `prepare-brief` requests `payments-repo-agent.repository-status`);
  returns attributed evidence without user-facing framing.

These three share the capability's I/O contract exactly. Only the
instructions/persona/delivery differ. The question is where that variation lives.

The decisive observation: **mode does not change the contract.** A concern that
never touches `inputSchema`/`outputSchema` has no business living on the capability
or on the universal binding. It belongs on the one skill whose *rendering* varies.

## Decision

1. **`variants` is an opt-in property of a skill.** A skill declares *either*
   `instructions` (unimodal — the default, unchanged, the common case) *or*
   `variants:` — a mapping of mode name to `{ instructions, contextFiles?,
   requiresContextTypes?, tools? }`. Skills with no modes are untouched and pay no
   ceremony.

   ```yaml
   kind: skill
   id: helio/skills/repository-status@1.0.0
   binds: helio/capabilities/repository-status@1.0.0
   inputSchema: '...'     # shared across variants
   outputSchema: '...'    # shared across variants
   variants:
     pipeline: { instructions: "Emit strict JSON, no prose; mark missing signals null." }
     manual:   { instructions: "Explain reasoning, surface risks, offer next action." }
     a2a:      { instructions: "Return attributed evidence for a calling agent." }
   ```

2. **The capability and the binding are unchanged.** No `mode` appears on the
   capability or on `SkillBinding`. A variant may set `instructions`,
   `contextFiles`/context references, `requiresContextTypes` (ADR-0013), and tool
   bindings (ADR-0017) — it may **not** override `inputSchema`/`outputSchema`.
   Forking the contract is the one thing mode must never do, and the loader rejects
   it.

3. **The resolver's slot/ambiguity model is untouched.** A multimodal skill still
   fills its capability slot exactly once; the promoted-name ambiguity check needs
   no special case. Only the **emitter** fans a multimodal skill into one artifact
   per variant. This is why this shape is preferred over a per-binding `mode`
   discriminator, which would have forced the ambiguity checker to special-case
   "same capability, different mode".

4. **Variants may narrow context and tool requirements — the governance hook.** A
   variant may raise `requiresContextTypes` to a stricter refinement (ADR-0013) and
   bind a differently-scoped tool (ADR-0017): the `a2a`/`pipeline` variants can
   *require* a governed context type and a governed-scope tool while `manual`
   accepts the base. This is where "personal reads the personal vault, a2a reads
   the governed one" is expressed as a **type constraint per variant**, not a
   hardcoded path.

5. **Variant selection is target-driven with an override.** Each target adapter
   declares the default variant for the invocation surface it represents
   (Copilot/Cursor → `manual`, MCP-callable/card → `a2a`, neutral/CI → `pipeline`),
   overridable by a build flag. A target consuming a unimodal skill is unchanged.

6. **Mode names are an open, author-defined vocabulary** with a recommended core
   set (`pipeline`, `manual`, `a2a`) that target adapters key their defaults on.

## Consequences

- Authoring stays DRY: one agent, one skill, N faces. The distinction lives in the
  type system, not in duplicated filenames.
- The common case (a skill with no modes) is entirely unaffected.
- **Cross-constraint with the on-disk format:** when TypeFerence source is written
  as frontmatter-plus-body (the deferred `.tfer` serialization, referenced in
  ADR-0013), "body = the skill's instructions" works only for a *unimodal* skill. A
  multimodal skill's per-variant instructions live in structured frontmatter; the
  body is either unused or carries the default variant. The format decision must be
  written aware of this.
- Work required: a `variants` field in the loader/model with the "no schema
  override" guard; emitter fan-out and per-adapter default-variant declaration; a
  spec section; a golden-file fixture exercising a multimodal skill across targets.

## Alternatives considered

- **Agent-per-mode.** Duplicates the entire agent to vary one skill — the sprawl
  TypeFerence exists to remove. Rejected as off-thesis.
- **`mode` on every `SkillBinding`.** Pollutes the ~90% of bindings that have no
  modes and forces the ambiguity checker to special-case same-capability
  multi-binding. Rejected.
- **`mode` in `inputSchema`.** Relocates the polymorphism to runtime, which the
  definition layer does not model. Rejected.
