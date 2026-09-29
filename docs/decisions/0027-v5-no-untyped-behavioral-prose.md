# 0027 — v5: no untyped behavioral prose

## Status

Accepted. Amends the specification to `schemaVersion: 5`. Depends on ADR-0026
(the closed frontmatter grammar); both land as one closure.

## Context

The language's thesis is that raw prose becomes a typed value with a declared
shape. Version 4 already enforced this almost everywhere: unknown fields are
hard errors through per-kind field maps, and non-whitespace bodies are legal
only on unimodal skills and contexts. Two doors remained open, and they were
the most organization-wide fields in the language:

- `workingNorms` on agents and profiles — a list of bare strings, each one
  free prose, appended into resolution by embedding order. The norms every
  agent inherits were the least-modeled content in the system: no type, no
  validation, no per-norm provenance beyond a repeated field marker, no way to
  attach structure such as scope or applicability, and no participation in the
  machinery contexts already have (dedup discipline, allow-lists, ambiguity
  checks).
- `description` on agents and profiles — a bare string that flows into
  emitted artifacts. Some emitters route it into model-facing surfaces.

Both predate user-defined context types (ADR-0013) and undercut the project's
own pitch.

## Decision

1. **Normative field-classification rule:** a field is exactly one of typed
   context, reference, or inert metadata; nothing else. Any field whose value
   reaches model-facing output must be a value of a declared, named type.
2. **`workingNorms` is deleted from every resource kind.** Normative prose is
   held context: an organization declares a contextType (for example a norm
   collection with `statement` text members), authors context resources of
   that type, and profiles/agents hold them via `context`. Embedding-order
   append-and-dedup semantics are preserved by existing held-context rules;
   provenance records each contributing profile or agent per context.
3. **`description` is reclassified as inert metadata.** It remains a plain
   string because it documents for humans; every emitter MUST place it only in
   human-facing manifest/bundle metadata and MUST NOT include it in
   model-facing instruction output. An emitter that cannot prove inertness
   drops the field.
4. **Agent-local context is not exempt.** Every context resource already
   declares a `contextType`; v5 closes the last inline channels instead of
   introducing new ones. The binding point (profile vs agent) does not matter;
   what matters is that inheritable prose always arrives as a typed,
   referenced resource rather than an inline string.

Rejected alternatives:

- **Deprecate `workingNorms`, keep accepting it**: preserves the untyped
  channel indefinitely and splits the corpus between two norm mechanisms.
- **Make `description` a typed context**: ceremony with no gain — the field's
  entire job is to be inert documentation; typing it would imply it participates
  in resolution.
- **Allow inferred/anonymous local types for small norms** ("just infer the
  shape"): an anonymous type is unnameable, unauditable, and would make digests
  depend on inference details. The honest escape hatch is raw Markdown outside
  the system, visibly.
- **Per-line norms inside skill bodies**: scatters governance across
  implementation files and loses composition dedup entirely.

## Consequences

- "A field is either typed context, a reference, or inert metadata" becomes a
  checkable property: closed field maps plus body-kind rules enforce it
  mechanically.
- Norms gain everything contexts already have: exact-ID addressing, versioning,
  embedding dedup, allow-list gating, ambiguity checks, per-resource evolution,
  and full provenance.
- Migration is mechanical: each former `workingNorms` entry becomes a member
  of a declared norm-collection context resource referenced from the declaring
  profile. Conformance fixtures carry the migrated shapes.
- The deletion is the claim under test for the deterministic setup wizard: the
  canonical onboarding suite must be authorable — by generator or hand — with
  zero legacy escape hatches. If the wizard cannot express ordinary
  organizational composition without an untyped field, that is a specification
  bug, not wizard scope.
