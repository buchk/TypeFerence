# 0015 — Exposure and visibility

**Status:** Proposed (2026-07-22)

## Context

"Authoring an MCP tool" has felt like a missing concept. It is not: **MCP is the
wire projection of a capability across the definition boundary** (ADR-0018), and you
never author an MCP — you author a capability and *decide whether it is a public
entry point*.

Most capabilities are internal composition plumbing: they exist to compose behavior
within an agent and nothing outside ever names them. A few are entry points meant to
be called by a consumer the definition does not control — another agent, a host
tool-loop, an external service. Today there is no way to say which is which, and the
existing C# `serve` exposed *every* skill as a tool, leaking internal plumbing as
public API.

This is precisely Go's exported/unexported distinction — the public-API boundary —
and it should be modelled the same way.

## Decision

1. **Visibility is a per-capability property: `internal` (default) or `exposed`.**
   Only `exposed` capabilities become part of an agent's public callable surface and
   are projected into callable-resource cards (ADR-0018). This is Go's export model:
   the boundary between public API and package-private plumbing.

2. **Exposure is declared at the definition and promoted through embedding**, the
   way Go promotes exported members of an embedded struct. An embedded profile's
   exposed capabilities are promoted onto the embedder's public surface; internal
   ones are not. This rides the promotion/ambiguity machinery the resolver already
   has — visibility is one added attribute on the promoted member.

3. **Default is `internal` (private-by-default), matching Go's unexported default.**
   Exposing mints a *stable public API* — a name and schema outside callers depend
   on — which is expensive to walk back; accidentally exposing all internal plumbing
   is a worse, less-reversible trap than accidentally leaving something callable
   internally. Private-by-default fails safe here. (Contrast the mutability axis in
   ADR-0016, which is *open*-by-default; the unifying rule is per-axis Go defaults:
   **open on override, private on export.**)

4. **Exposure is a commitment, surfaced by tooling.** Because exposing is minting
   public API, `inspect`/the LSP show which capabilities are exposed, and `diff`
   flags an exposure change as the API-surface change it is.

## Consequences

- The only new authoring surface is a one-bit visibility marker; the default (all
  internal) means an agent exposes nothing until deliberately opened.
- Callable-resource cards (ADR-0018) carry exactly the exposed capabilities — no
  more blanket dumps of internal skills.
- Exposure composes through embedding without new machinery.

## Alternatives considered

- **A central `exposes:` list on the agent.** Less Go-native than
  visibility-at-definition-plus-promotion, and it fragments the public surface away
  from where members are declared. Rejected in favor of the promotion model.
- **Expose-by-default.** Turns internal composition into public API by omission — the
  expensive, irreversible failure. Rejected.
