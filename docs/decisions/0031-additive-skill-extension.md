# 0031 — Additive skill extension

## Status

Proposed (2026-09-29). On acceptance: adds `extends` and skill-level `sealed`
to the version 6 skill document. Narrows ADR-0016 for skills: skill-level
sealing forecloses extension. Companion to ADR-0029 and ADR-0030.

## Context

Teams specialize common skills. Version 5 can only rebind a capability to a
different skill, and the replacement cannot reuse the base's instructions. In
`examples/helio`, `payments-repository-status` can only gesture at its base in
prose ("Apply the repository-status capability…") and hope the model supplies
the base behavior; its alternative is to restate it. Across an organization
that becomes dozens of near-copies of each core skill, drifting independently,
and a fix to the core reaches none of them.

Skills ship as SKILL.md files in a host pool keyed by name (ADR-0029), so
whatever sharing exists must be resolved at build time, and every emitted name
must denote one body.

## Decision

1. **`extends`.** A skill may name exactly one base skill. Chains are allowed;
   cycles are errors.
2. **Same contract.** An extension inherits its base's capability and schemas.
   A skill declares `binds` or `extends`, never both. An extension may restate
   its base's schemas, but different schemas are an error. Binding an
   extension where its base's capability is already bound rebinds that
   capability (ADR-0030 decision 4).
3. **Additive instructions.** The emitted instructions are the base's
   flattened instructions, verbatim, followed by the extension's own,
   separated by exactly one blank line. Per mode:
   - base and extension both unimodal: one concatenation;
   - one side unimodal: its text applies to every mode of the other side, and
     the multimodal side's modes are the result's;
   - both multimodal: the extension may supply text only for modes the base
     defines; for each base mode, base text then extension text (or base text
     alone).
   An extension with no text of its own keeps its base's text: it renames the
   base with its own description, context, and requirements. Replacing or
   removing base text is not expressible; that is a new skill.
4. **Requirements accumulate.** `requiresTools`, `requiresContextTypes`, and
   skill-held `context` are the union along the chain, base first, deduplicated
   in first-seen order. An extension of an agent-dependent skill is
   agent-dependent (ADR-0030 decision 6).
5. **Own identity and routing.** An extension has its own name, which is its
   emitted SKILL.md name and slash command, and its own required
   `description`.
6. **Sealed skills cannot be extended.** `sealed: true` on a skill document
   forbids any skill from extending it. Appended text can countermand the
   base, so extension modifies the member; this is consistent with ADR-0016's
   rule that sealing forecloses modification. Binding-level `sealed` keeps its
   meaning, and because an extension shares its base's capability,
   substituting an extension for a sealed binding is already a rebind error.
7. **One name, one body.** A SKILL.md is a function of its extension chain and
   the plugin's mode only; agent-held context never enters it. An emitted name
   therefore denotes one body per mode across every plugin in a build.
8. **Pack compatibility is computed.** Skills that implement one capability
   form a family: every member of an extension chain shares its root's
   capability, and skills binding one capability document are a family too.
   Two plugins that ship different members of one family compete for the same
   requests when installed together. Build reports every family with more than
   one emitted member, per mode, in `.typeference/compatibility.json` at the
   agent-plugin target root, and the CLI prints each conflict after a build.
   The report is advisory; installing one plugin per family resolves it.

## Consequences

- A team specialization is a few lines against a shared core, and a core fix
  reaches every team on the next build.
- Installed pools stay coherent: no two installed skills share a name, and
  "install one team pack at a time" becomes a checked property rather than
  advice.
- Conformance needs fixtures for flattening, the mode-merge cases, sealed
  extension, a changed contract, and a cycle.
- `examples/helio`'s `payments-repository-status` becomes an extension of
  `repository-status`.

## Alternatives considered

- **Multiple bases.** Deferred: ordering several bases' prose reintroduces
  ambiguity rules into instruction text.
- **Section-level replace or override.** Rejected: a replaceable base cannot
  guarantee anything; a changed base is a different skill.
- **Runtime composition** ("first follow the core skill, then…"). Rejected:
  depends on model compliance and cannot be verified at build time.
- **Keep specializations as full copies.** The status quo that motivated this
  record.
