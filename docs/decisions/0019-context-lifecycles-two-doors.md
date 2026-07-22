# 0019 — Context lifecycles: the two doors

**Status:** Proposed (2026-07-22)

## Context

A knowledge vault (Obsidian or otherwise) relates to a TypeFerence agent in two
distinct ways, and conflating them breaks either determinism or governance. The two
sit on opposite sides of TypeFerence's most fundamental line — definition vs.
deployment ("no deployment state, hosted runtime, or model credentials" in the
definition layer).

## Decision

1. **Door A — compile-time context (baked in).** A context object (ADR-0013, an
   instance of a `contextType`) exists as source and is compiled *into* the agent's
   artifacts. Its content is part of the digest: reviewable, diffable, governed.
   TypeFerence fully owns Door A — it reads the object and materializes it.

2. **Door B — runtime context (queried live).** The compiled agent declares a
   capability whose fulfillment is an external **tool** (ADR-0017); at execution the
   *host* reads the live, mutable, possibly-private vault through that tool.
   TypeFerence types only the contract — the `contextType` *shape* the data must
   satisfy, the tool's signature, and the scope binding — and the content is **never**
   in the digest. TypeFerence *describes* the access; it does not *perform* it. The
   vault-reading tool body is **user-written** (the same boundary as "embeds no LLM
   provider").

3. **One vocabulary, two lifecycles.** The same `contextType` (e.g.
   `castOfCharacters`) is the invariant *shape* whether baked (A) or queried (B). The
   split is a *relationship* — "is this content in the definition, or queried by the
   deployment?" — not necessarily a folder split; a single note's *shape* can be
   authored at design-time (A) while its *content* is read at run-time (B).

4. **What TypeFerence ships for Door B: the contract, never the connector.** No vault
   reader, no Obsidian runtime, nothing to maintain. `obsidian-vault-lookup` and
   friends are *user-written* tools conforming to a typed contract TypeFerence emits.

## Consequences

- Determinism is protected: runtime content is never baked into a deterministic
  artifact, so it can't leak into or destabilize the digest.
- Governance is protected: definitional content stays committed, reviewable, and
  diffable rather than living as live external state.
- Scope stays honest and small — TypeFerence types the *what* (shape, contract,
  scope) and stays out of the *how* at runtime.
- This is the lifecycle counterpart to the product framing: the vault is the informal
  substrate; formalizing a slice *into* a definition is Door A, and typing a running
  agent's *access* to the substrate is Door B.

## Alternatives considered

- **Treat the runtime vault like compile-time context (bake it in).** Bakes live,
  mutable, possibly-private data into a deterministic artifact — kills determinism,
  leaks data, couples the definition to transient state. Rejected.
- **Treat compile-time context as a live resource.** Definitional content would live
  as external state, losing reviewability and governance. Rejected.
- **Ship a built-in vault/Obsidian connector.** Puts TypeFerence in the runtime
  business it deliberately stays out of. Rejected; Door B tools are user-written.
