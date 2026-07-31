# 0014 — Go-only implementation; the spec is the open invitation

**Status:** Accepted (2026-07-23)

Supersedes the dual-implementation commitments in ADR-0003, ADR-0004, and
ADR-0005 (the canonicalization rulings and the fixture corpus survive; the
requirement that *two* implementations agree does not).

## Context

TypeFerence began as two implementations — a C# reference (`src/`) and an
independent Go implementation (`go/`) — held to byte-identity by a shared
conformance suite (ADR-0005). C# came first because it was the author's more
familiar language; the model being compiled (Go-like embedding, structural
interfaces, promoted-name ambiguity) is Go-shaped. The C# implementation also
carries the only `serve` (MCP runtime) surface.

Two implementations proved the specification is *normative* rather than a
description of one program's behavior. But that credential serves a constituency —
*other people who implement TypeFerence* — that does not currently exist, and it is
paid for on **every feature**: every change is built twice and held byte-identical.
The layer TypeFerence targets is still up for debate, and velocity on the tool
(compiler, LSP, `.tfer` tooling, Obsidian integration) is what earns adoption. None
of that is served by a second implementation.

The decisive distinction: **conformance is not determinism.**

- *Determinism* — one compiler produces byte-identical output from identical input —
  is what makes `diff` meaningful, the committed-digest demos real, and governance
  possible. It is a property of one good compiler.
- *Conformance* — two independent implementations agree — is the part that serves
  the non-existent implementer constituency.

Dropping the second implementation loses only the latter.

## Decision

1. **Go is the sole implementation.** Retire the C# reference implementation
   (`src/`, `tests/`, `TypeFerence.slnx`, `Directory.Build.props`) and the C# CI/
   conformance jobs. One codebase, full velocity.

2. **The specification remains the source of truth.** `docs/specification.md` stays
   normative *in principle* — "here is the abstraction; realize it differently if
   you want, but this repository's Go compiler is the one living answer." This keeps
   the standard door open (a future second implementation could reappear) without
   staffing it now, and preserves the accurate framing that TypeFerence is a *spec
   with a reference implementation*, not merely a program.

3. **The conformance suite becomes a Go golden-file determinism suite.** The
   `conformance/fixtures/` corpus and canonicalization rulings (ADR-0004) survive
   unchanged; the suite is re-pointed to prove "the Go compiler still emits the
   committed bytes" instead of "Go and C# agree." The digest guarantee — the
   property that matters — is fully retained.

4. **`serve` is not ported (ADR-0018).** It is off-thesis (deployment/runtime), and
   its value is reframed as *emitting a static callable-resource card*, not running
   a server. Its retirement with C# costs the core nothing.

## Consequences

- Full-velocity single codebase; every subsequent ADR in this batch is implemented
  once, not twice.
- Determinism, digests, `diff`, and provenance are unaffected.
- The `.slnx`/.NET toolchain requirement disappears from build and CI.
- The one concrete loss is `serve` as a working runtime; superseded by the
  callable-card emission (ADR-0018).
- The spec is downgraded from "normative, proven by two implementations" to
  "normative in principle, realized by one." Recoverable if a second implementation
  ever returns; deliberately reversible.

## Alternatives considered

- **Keep dual-impl.** Justified only if TypeFerence's ambition is a *standard others
  implement*. For a *tool people install*, it is a recurring tax on the wrong axis.
  Rejected in favor of the enablement-first posture.
- **Collapse to "a Go package," drop the spec.** Fastest, but bricks the standard
  door and downgrades the spec to "whatever the Go code does." Rejected: keeping the
  spec doc costs almost nothing and preserves optionality.
- **Port `serve` to Go.** The most expensive port for the least critical-path value,
  and off-thesis regardless. Rejected (ADR-0018).
