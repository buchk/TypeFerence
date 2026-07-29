# 0016 — Sealing: mutability and presence

**Status:** Accepted (2026-07-23)

## Context

A security (or platform) team authors a profile that others embed. Some of its
skills are meant to be extension points others may override; others are guarantees
that must survive composition unmodified — if you mix in the safety profile, you use
its safety control as-is. Go's model does not support this: embedding promotes, but
an embedder may *shadow* a promoted member by redeclaring it. There is no `final`.
That is exactly what a governance foundation cannot allow — any embedder could
silently shadow the safety control and its guarantee would evaporate.

This is the first place the Go aesthetic genuinely breaks: Go trusts the programmer,
so it has no `final`; a governance-composition layer cannot, because the whole point
is that one team's control survives another team's mix-in intact.

## Decision

1. **`sealed` modifier (mutability axis: `open` default / `sealed`).** A sealed
   capability/skill may not be overridden, rebound, or suppressed by an embedder;
   attempting to is a compile error. Crucially, **sealed forecloses *modification*,
   not *extension*.** It is a `final` *member* on an *open* container: the profile
   stays embeddable, you may add adjacent capabilities freely, you simply cannot dig
   under the guaranteed member. Sealed member ≠ closed container.

2. **The three operations sealed governs:** *add adjacent behavior* (always
   allowed — this is extensibility, intact); *override/rebind the sealed capability*
   (compile error); *remove/suppress it* (compile error).

3. **`required` modifier (presence axis: `optional` default / `required`).** A
   required capability must be *bound* in every concrete agent that carries the
   mandate. Because embedding only ever promotes — nothing in the model can drop
   a binding — "cannot be dropped" is not where this axis earns its keep. The
   operation it governs is the opposite one: **a mandate may be declared without
   a fulfillment.** A binding that omits `ref` and names a `capability` is an
   **abstract requirement**: the profile demands the capability, an embedder
   supplies it. A ref-less binding is meaningless unless `required`, so that
   combination is rejected rather than silently ignored.

   Enforcement is asymmetric by kind, which is what makes profiles abstract:
   a **profile** MAY carry unfulfilled requirements; an **agent** is concrete and
   MUST bind every capability mandated by itself or anything it embeds, or
   composition fails. Requirements propagate through intermediate profiles.

   `required` is *presence only*: overriding a required-but-open capability is
   legal, since the capability is still present. What fills it is `sealed`'s
   concern. `sealed + required` is therefore the mandatory control: must be
   carried *and* cannot be modified.

   This is the demand side of the type system, which nothing else supplied:
   `satisfies` is inferred structurally, so interfaces only *observe* whether a
   capability is present. `required` is the only primitive that can *require* it.

4. **Open-by-default, enablement-first.** Everything is `open` unless explicitly
   `sealed`. The people who need sealing (governance authors) are exactly the ones
   who will remember to seal; open-by-default keeps ordinary composition frictionless
   and keeps the divergence from Go *itself* opt-in — the type system is 100%
   Go-shaped until someone explicitly reaches for the one primitive Go lacks.
   (Combined with ADR-0015: per-axis Go defaults — **open on override, private on
   export**.)

5. **No special root.** A profile is a profile; "root" is only graph position
   (embeds nothing / is embedded), not a distinct kind. Sealing is a uniform
   modifier available on any profile/skill, never a foundation-only feature.

6. **Sealing later is a breaking change; make it observable, not default.** Going
   `open → sealed` can break embedders who were overriding. This is managed by
   *visibility of seal state* — `inspect`/the LSP show sealed vs open, and `diff`
   flags the transition — not by a stricter default.

## Consequences

- Governance foundations become real: a control can be handed to embedders that they
  may build on but not weaken, enforced at compile time.
- Enablement stays the default; security is *achievable and observable* without
  taxing ordinary composition.
- Some compositions become **illegal, by design**: two profiles that seal the same
  capability to *different* skills cannot be embedded together (a hard error that
  surfaces a real governance conflict). A sealed diamond (the same sealed member
  reached by two paths) is conceptually fine — one member, no conflict — but note
  the resolver's pre-existing same-depth promotion check currently rejects *any*
  equal-depth diamond (sealed or not) with an ambiguity error before sealing is
  considered; lifting that limitation is separate work.
- Three orthogonal axes now exist: visibility (ADR-0015), mutability, presence — all
  permissive by default, every lock opt-in.
- Profiles gain an abstract-base-class shape: a governance profile can hand down an
  obligation ("every agent must bind an audit control") without dictating the
  implementation, which `sealed` alone cannot express — sealing protects a control
  you supply, `required` demands one you do not.
- Modifier state is emitted into bundles when a lock is engaged (absent means the
  axis is at its permissive default), so `inspect` can show it and `diff` can flag
  an `open -> sealed` or `optional -> required` transition.

## Alternatives considered

- **Sealed-by-default for security foundations (fail-closed).** Protects authors who
  forget to seal — but those are exactly the authors who don't need protection, and
  it taxes every ordinary profile and breaks the uniform "no special root" rule.
  Rejected in favor of open-by-default plus seal-state observability.
- **No sealing (stay pure Go).** Leaves governance guarantees unenforceable under
  composition. Rejected; this is the deliberate, minimal divergence.
