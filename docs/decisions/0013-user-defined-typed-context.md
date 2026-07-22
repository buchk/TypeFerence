# 0013 — User-defined typed context

**Status:** Proposed (2026-07-22)

## Context

Context today is untyped. `contextFiles` is a list of bare relative paths, and the
loader only checks that each path exists and does not escape the source root
(`go/internal/resource/loader.go`). Nothing distinguishes a principal-preferences
note from a safety policy from a "cast of characters" roster — they are all just
markdown at a path. It is also the one place the type system reaches *outside*
itself: a raw filesystem pointer to an opaque blob, the assembly-language escape
hatch inside the higher-level language.

Two things are missing:

1. **Naming a *kind* of context** — "cast of characters," "data-classification," a
   particular vault's note shape — without that kind being a hardcoded TypeFerence
   feature. TypeFerence ships the type *system*, not the types.
2. **Typing the boundary between a component and the knowledge it carries**, so a
   mismatch is a build-time error rather than a runtime surprise.

The clarifying frame: context is the **fields** half of the object model
(capabilities/skills are the methods; ADR-0015/0017). An agent *holds* typed
context; a skill *requires* the context types it needs to operate.

## Decision

1. **New resource kind `contextType`** — a first-class, versioned, user-defined
   type. `id`, `displayName`, `description`, and an optional `schema` (JSON Schema
   over the note's frontmatter). With `schema`, conformance is checked
   structurally; without it, the type is a name tag (still useful for the gating in
   decisions 4–5). "Cast of characters" and a vault's note shape are thus *defined
   in YAML by a user*, never a language feature.

2. **Context types embed/refine structurally, like interfaces.** A
   `governedCastOfCharacters` may embed `castOfCharacters` and add provenance/owner
   frontmatter; a governed roster *is a* cast of characters (satisfies the base).
   This reuses the interface satisfaction machinery and is what makes a **trust
   boundary a type** rather than a path: a variant or capability that requires the
   governed refinement structurally rejects a bare personal note (ADR-0012 §4).

3. **Agents/profiles HOLD context; skills REQUIRE context types.** Holding typed
   context is agent/profile state (`context:`, formerly `contextFiles`). A skill
   declares the types it needs:

   ```yaml
   # skill
   requiresContextTypes: [acme/context-types/my-obsidian-shape@1.0.0]
   ```

   The resolver checks that the holding component supplies context satisfying the
   skill's required types — the same provide/require/check shape capabilities
   already have. This is a *wiring* constraint, not a contract change: two skills
   requiring different context types remain substitutable at their capability.

4. **Governance gating.** A profile/agent/capability may declare
   `allowedContextTypes` (a whitelist; anything outside it — direct, promoted, or
   slot-filled — is a build error) and/or `requiresContextTypes` (must be present
   after composition). Allow-lists **intersect** through embeds (the most
   restrictive ancestor wins; embedding narrows, never widens).

5. **Reference context by id, not by path.** `contextFiles: [context/x.md]` is
   deprecated in favor of `context: [acme/notes/x@1.0.0]`, resolved through the
   resource map like `embeds`/`skills`. This closes the last raw-path reference: a
   context object is a first-class resource (`kind: context`) with an `id`, a
   `contextType`, typed fields, and prose. Typed slots accept a context type, so
   "fill the `vault` slot" becomes "fill it with a note of type X".

6. **Types come from the type system, not the substrate.** Frontmatter/field values
   are interpreted **schema-directed** against the declaring `contextType` — so a
   `country: no` field is the string `"no"`, never the boolean, and YAML's implicit
   coercions never fire. Determinism holds: contextType ids sort into the canonical
   key space, field values canonicalize before hashing, and provenance records
   which component contributed each typed context.

## Consequences

- "Cast of characters," "data-classification," and a vault's shape become things a
  user *defines*, not things TypeFerence *has*.
- Context joins the type system fully — no raw path references remain.
- A real governance primitive: a profile or capability can constrain or require the
  *kinds* of context flowing into anything built on it, the context analogue of
  structural capability typing.
- The zero-ceremony default is preserved: untyped context keeps working; every new
  field is opt-in.
- **The on-disk serialization is deferred.** How a context object is written on
  disk (frontmatter-plus-body, a distinct `.tfer` extension, schema-directed
  parsing, verbatim body, and whether Obsidian authors it natively) is a separate,
  currently-parked decision — the *type semantics* here do not depend on it. The
  reference-by-id and schema-directed-typing rulings above are the load-bearing
  parts; the file format ADR will follow when the extension question is settled.
- Compile-time typed context is *Door A* (baked into artifacts); a context *type*
  may also describe data a tool reads at runtime (*Door B*), where TypeFerence types
  the shape but does not ship the reader (ADR-0019).

## Alternatives considered

- **Hardcode a fixed context taxonomy.** The exact anti-pattern the project rejects.
  Ship the type system, not the types. Rejected.
- **Reference-side typing only** (`{ path, type }`, no id). Keeps the raw path and
  lets a note be silently retyped per reference. Rejected in favor of first-class
  context resources referenced by id.
- **Type context purely nominally.** Kept as the zero-schema case, but the optional
  `schema` is what makes `requiresContextTypes` more than a naming convention.
