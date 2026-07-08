# 0006 — Self-hosting the maintainer agent, and what it revealed

**Status:** Accepted (2026-07-08)

## Context

The agent that maintains this repository is now defined in TypeFerence's own terms
under `agents/maintainer/` (four profiles: spec-conformance, determinism-guardian,
trust-signing, contribution-workflow; two real capabilities: verify-conformance and
audit-drift). The repository-root `AGENTS.md` and `dist-maintainer/` are build
artifacts of that definition (kept outside `dist/`, which remains purely the
committed reference output of `examples/helio`); CI (`selfhost-drift` job)
recompiles the definition and fails on any byte of drift. Provenance back to the
canonical source digest is carried by `dist-maintainer/ard/ai-catalog.json`
(source-package digest plus `derivedFrom` links), using the spec's own optional ARD
emission rather than an ad-hoc mechanism.
The publisher domain is `typeference.example` — a reserved documentation TLD, chosen
deliberately so the artifact cannot be mistaken for a real published catalog.

Self-hosting was also a design probe: every place the type system could not express a
real maintenance constraint and prose had to carry the weight is recorded here as
design feedback, not papered over.

## Design feedback — where the type system fell short

1. **The source-root sandbox blocks references to the governed artifacts.** Context
   files and slot paths must exist beneath the source root, so the maintainer
   definition cannot reference `docs/specification.md`, `docs/decisions/`, or
   `conformance/` directly — the very files its norms govern. The context files
   restate those paths as prose. A typed, read-only "repository reference" (path +
   expected digest, excluded from promotion) would make these links checkable.
2. **Norms cannot express conditional or procedural rules.** "Semantic changes land
   in the spec *before* either implementation" is an ordering constraint;
   `workingNorms` are unordered deduplicated strings. All sequencing lives in prose
   (`context/spec-workflow.md`). The framework has no notion of a workflow or a
   precondition.
3. **Invariants are not a type.** "The signature map stays outside the source root"
   is enforced by both compilers, but the maintainer definition can only *assert* it
   as a norm; there is no way to declare an invariant that tooling could check
   against the definition itself. Capabilities model invocable actions, not
   properties that must always hold.
4. **Context content is invisible to the neutral target's drift surface.** Neutral
   bundles reference context files by path without embedding content, so editing a
   context file changes no neutral artifact byte. The drift gate only catches
   context edits because it also emits the ARD source package (which inlines file
   content and the source digest). Without ARD emission, `AGENTS.md` could be
   "current" while its context silently changed. A content digest for referenced
   context files inside `bundle.json` would close this gap at the core level.
5. **No tie between definition version and repository state.** The agent is
   `typeference/typeference-maintainer@0.1.0`, but nothing relates that version to a
   commit. The ARD source digest identifies the definition bytes, not the repository
   they govern.

These are recorded as observed limitations. None is fixed in this change set;
each would be a spec change with its own ADR and fixtures.

## Consequences

- `make selfhost` regenerates the artifacts; `make selfhost-check` and the CI job
  reject drift. Hand-editing root `AGENTS.md` is now a broken build.
- The maintainer definition doubles as a real-world usage example of profiles,
  promotion, and capabilities beyond the fictional `examples/helio`.
