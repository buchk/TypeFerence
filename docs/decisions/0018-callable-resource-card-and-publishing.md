# 0018 — Callable-resource card and publishing

**Status:** Accepted (2026-07-23)

## Context

The C# `serve` verb stood up an MCP server from compiled agents, turning each skill
into a live tool whose calls returned a deterministic invocation package. ADR-0014
retires it as off-thesis runtime. But its *value* is not the running process — it is
a **packaging** decision, and that value should be recovered as a deterministic
emission rather than a server.

Meanwhile `--emit-ard` already publishes ARD catalog entries — a source-package
entry and a precompiled-bundle entry per concrete agent/target — but it *explicitly
punts* on callable resources: the README notes that callable MCP/A2A resources
"require their own deployed endpoint and native card." `serve` assembled exactly that
native card in memory and then threw it away by only serving it live. The two nearly
conflict; in fact they complete each other.

The design boundary is firm: **no ARD registry lifecycle, federation, dependency, or
deployment metadata in core semantics.** So emission is core; anything that *pushes*
to a registry is the deployment edge.

## Decision

1. **`serve`'s output becomes a static callable-resource card — the third ARD
   archetype.** `--emit-ard` now emits three entry kinds: **source-package**
   (existing; the design-time package feed a profile is fetched from to embed),
   **precompiled-bundle** (existing; drop-in native artifact), and
   **callable-resource card** (new; the fully-assembled invocation contract — exposed
   capabilities per ADR-0015, their schemas, the instruction-package template, and
   `derivedFrom` provenance). The card is static and deterministic; *answering* calls
   is an optional thin host, not core.

2. **You never author an MCP; MCP is a projection.** A capability projected *outbound*
   across the boundary becomes a callable card (MCP tool / A2A card); a capability
   required *inbound* is fulfilled by an external tool (ADR-0017, Door B). Both flatten
   into "an MCP tool" on the wire; the skill/tool distinction is TypeFerence-internal.
   MCP crosswalk (from ADR-0017): our `capability` → MCP **tool**; MCP **"capabilities"**
   is transport-layer negotiation, unrelated to our `capability`.

3. **`--emit-ard` defaults on when a publisher identity is configured.** ARD emission
   needs a publisher domain, so it cannot hard-default; instead it is on whenever a
   publisher identity is resolvable (flag or project config, e.g.
   `typeference.trust.yaml`) and cleanly skipped with a hint otherwise. "The default
   build output is catalog-ready" holds for projects that have declared who they are,
   without failing a first-run `build` for a tire-kicker.

4. **Publishing is an edge verb, not core.** The vocabulary chain is `build` → a
   *catalog entry* → **`publish`** → a *registry*. Because pushing to a registry is
   registry lifecycle (disclaimed by core), `publish` is an optional edge subcommand
   (or an external registry tool consuming the emitted entry). Core *emits* the
   registerable artifact deterministically; the network write is the edge. "Compile to
   ARD as it exists" and "register to ARD as it exists" both **preserve the boundary**:
   TypeFerence is a conformant *client* of an external standard, never the author of
   registry lifecycle.

## Consequences

- The callable interface `serve` produced is now a first-class, static, registerable
  artifact — exactly what an ARD-based agent finder needs (a card, not a live endpoint
  to crawl).
- Porting an MCP server to Go (expensive, runtime, off-thesis) is replaced by emitting
  a card (cheap, deterministic, on-thesis).
- `serve` the verb effectively evaporates into `--emit-ard`; a live host demotes to an
  optional `host <card>` convenience that may be written later or never.
- The determinism/core boundary is respected: emission is deterministic; publication
  is an explicit edge.

## Alternatives considered

- **Keep `serve` as a runtime.** Off-thesis and the most expensive thing to port
  (ADR-0014). Rejected in favor of emission.
- **Put `publish` in the deterministic core.** Would pull registry lifecycle into core
  semantics, which the design boundary disclaims. Rejected; `publish` is an edge.
- **Emit an MCP-server config as an output format.** Conflates transport with the
  packaging decision; the card is the right, transport-neutral layer (bindable to MCP
  or A2A). Rejected.
