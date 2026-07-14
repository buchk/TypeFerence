# Design note: typed context shapes

Status: idea captured 2026-07-13, not designed, not scheduled. A future ADR
decides; this note only pins the problem and the hook points so the thought
survives.

## Problem

v3 disciplines composition: profiles, capabilities, skills, and overrides are
typed, versioned, promoted with ambiguity checks, and carry provenance. But
`contextFiles` are opaque markdown. Everything the type system squeezed out of
instruction files can silently migrate into referenced context files —
org charts, timelines, policies, and de-facto behavioral overrides living as
free prose that no compile-time check can see. The sprawl TypeFerence exists
to eliminate does not disappear; it relocates to the one untyped surface.

## Gesture at a solution

1. **Governable context types.** A small starter registry of shapes a context
   file can declare itself to be — e.g. `cast-of-characters` (roster: names,
   roles, authority), `timeline` (ordered events; **decision points** as
   first-class entries on a timeline, not a separate type), `glossary`,
   `policy` (addressable clauses). Declaring a type is optional; *using* one
   is strict: the compiler validates the file against the shape and fails
   closed on drift, same as every other v3 check.
2. **Extensible registry, strict instances.** Do not enumerate the world's
   context types — runtime-adjacent shapes will keep appearing. Make adding a
   type cheap (a shape schema is itself a versioned resource a source tree
   can carry), but make conformance to a chosen type non-negotiable. Strict
   about use, liberal about the universe.
3. **Overrides belong in YAML, not prose.** If an agent deviates from an
   embedded profile's behavior, that override should be a declared member on
   the typed resource — baked into the compiled skill/bundle with provenance —
   never a paragraph in a context file that quietly contradicts what the
   profile promoted. Possible compile-time stance: typed context files simply
   have no vocabulary for behavioral instruction, which is what makes them
   governable.

## Hook points when this becomes real

- Loader/validation: `resource.validateShape` and friends already own
  referenced-file checks; typed context validation slots beside them.
- Spec: a "Typed context" section; canonical form questions (ordering,
  formatting) get the ADR-0004 treatment so digests stay stable.
- Conformance: new fixtures per shape, including failure fixtures.
- ARD: each context type maps naturally to a media type on published entries.
- Self-hosting: `agents/maintainer/context/*` is the first test corpus —
  ADR-0006 already catalogs where its prose is doing type-system work.
