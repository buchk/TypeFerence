# 0030 — Plugin documents, thin agents, and v6 authoring

## Status

Proposed (2026-09-29). On acceptance: amends the specification to
`schemaVersion: 6`; changes resource identity, source membership, and the
per-kind field maps. Companion to ADR-0029 (Agent Plugins target), ADR-0031
(additive skill extension), ADR-0032 (the v6 grammar), and ADR-0033 (import).

## Context

ADR-0029 makes an installable plugin the output. The unit people install is a
curated bundle — several agents plus a collection of skills — and the same
skill or agent belongs in many bundles. Version 5 has no document for that
unit: every agent is emitted, and nothing states which agents and skills ship
together.

Version 5 is also expensive to author. In `examples/helio`, one agent with one
specialized skill spans an agent, profiles, a capability, a skill, and context
documents, each repeating `schemaVersion`, `kind`, and a versioned `id`, with
the capability's schemas copied byte-for-byte into the skill. Contributors
today are power users; the aim is to widen that group.

Go-style composition stays the model: a profile may stand alone and reference
skills from any layer; an agent may embed another agent and change one or two
bindings, or change nothing and simply assemble job-shaped profiles.

## Decision

1. **Plugin documents (`*.plugin.tfer`).** A plugin is the solution file for
   one installable package. It links; it does not compose. Fields:
   `description` (required); `agents`; `profiles` (ship a profile's resolved
   skill set without an agent); `skills` (skills shipped directly); `modes`
   (`manual` default, `pipeline` optional, ADR-0029). No `embeds`, `context`,
   or bindings. A plugin with no agents is a skills pack. A plugin's name is
   its path's leaf.
2. **The manifest lists plugins and exports; membership is the closure.** The
   project manifest gains `plugins` and `exports`. Source membership, and so
   the source digest, is the transitive closure of references from them.
   Unreferenced files are not source members and are never parsed. Exports are
   the documents other packages may reference; a package with no plugins is a
   library.
3. **Identity from paths.** A document's kind comes from its file suffix, its
   identity from the manifest name plus its root-relative path, and its version
   from the manifest. Resource documents carry no `schemaVersion`, `kind`, or
   `id`. A reference is a root-relative path within a package, or
   `<package>:<path>` into a direct dependency, which must declare the package
   and export the document. Each field checks the kinds it accepts by suffix.
   A moved file breaks every reference to it; each is a build error.
4. **Implied capabilities and path bindings.** A skill that neither binds nor
   extends a capability defines one, identified by the skill, with `internal`
   visibility; an extension inherits its base's (ADR-0031). A binding in an
   agent or profile is a skill path, or a mapping of `skill`, `capability`,
   `required`, and `sealed`; a capability reference may name a skill, meaning
   that skill's capability. Binding a skill whose capability an embedded layer
   already binds rebinds it under the existing shallowest-wins, ambiguity, and
   `sealed` rules. Capability documents remain for schemas shared by unrelated
   skills and for exposure, since an implied capability is never exposed.
5. **Thin agents.** An agent is identity (`description`, `displayName`),
   objectives, and composition (`embeds`, `skills`, `context`). Its body is its
   objectives: a value of the built-in `text` context type carried by the
   agent itself, rendered after its title, and inherited through embedding
   (embedded agents' objectives first, each source once). Knowledge a whole
   team needs is context the agent or its profiles hold. A context document
   without a `contextType` is a value of the built-in
   `typeference/builtin/text@1.0.0`, so a prose context is just its body; the
   type is named, so ADR-0027's no-untyped-prose rule holds.
6. **Independent and agent-dependent skills.** Skills may hold context
   (`context` on the skill), which ships in their SKILL.md. After flattening
   its extension chain (ADR-0031), a skill is, per mode:
   - **independent** when its own held context satisfies every context type
     it requires. Its SKILL.md is self-contained: it works when invoked
     directly as a slash command, and it may ship through a plugin's `skills`
     or `profiles`;
   - **agent-dependent** otherwise. The composing agent must hold satisfying
     context (the existing v5 check, now also counting the skill's own). It
     ships only through an agent that binds it; shipping it through `skills`
     or `profiles` is an error. Invoked directly outside its agent it lacks
     that context — documented behavior, not a hidden failure.

   The class is derived from requirements and held context, never declared.
7. **Shipping profiles directly.** A profile listed in a plugin's `profiles`
   must be concrete (no unfulfilled `required` binding) and must hold no
   context, directly or through embedding: without an agent there is nowhere
   to deliver it.
8. **No migrator.** Nothing outside this repository uses version 5, so no
   `typeference migrate` ships. The repository's examples, maintainer
   definition, and playground starter were converted by hand, and the setup
   wizard's generator (ADR-0028) now emits version 6.
   Product entry points accept only version 6; the historical conformance
   corpora stay reproducible through archival loaders used only by the
   conformance runner. Existing GitHub Copilot content enters through
   `typeference import` (ADR-0033).

## Consequences

- Beyond the project manifest, a plugin with one agent and one skill is three
  small documents with no hand-typed identity: the plugin, the agent, and the
  skill.
- Promotion is a one-line change: link an existing agent or skill into another
  plugin. Nothing moves.
- Per-team plugin documents map onto CODEOWNERS; the manifest and shared
  profiles belong to the platform owners.
- Membership by closure means the source digest covers exactly what shipped,
  and a half-written draft beside the sources cannot break a build.
- Path identity trades rename stability for zero ceremony; the compiler
  reports every broken reference.

## Alternatives considered

- **Infer layers from directories** (each directory a profile or agent).
  Rejected: couples organizational structure to the filesystem and makes
  membership implicit.
- **One plugin per agent.** Rejected: the installable unit is a bundle.
- **Plugin entries inline in the manifest.** Rejected: every team would edit
  one file, blurring ownership and multiplying merge conflicts.
- **Keep explicit versioned IDs in every document.** Rejected: ceremony with
  no consumer; the manifest already versions the package.
- **Globs in plugin documents.** Deferred: explicit lists keep "this skill now
  ships in this plugin" a reviewable one-line diff.
- **Let a plugin carry pack-wide context for its skills.** Rejected: team
  context belongs in the team's agent; skills stay either self-contained or
  explicitly agent-dependent.
- **Declare skill independence with a flag.** Rejected: a flag can disagree
  with the requirements; deriving the class cannot.
- **Ship a v5-to-v6 migrator.** Rejected: it would have exactly one user, this
  repository, and would outlive its purpose.
