# 0021 — Restore exact, locked source packages

**Status:** Accepted (2026-07-29)

## Context

Composition currently resolves only one source directory. An enterprise profile
such as `marathon/enterprise-foundations` must be copied into each repository or
left unresolved. Real organizations already publish internal artifacts through
JFrog, Azure Artifacts, HTTP servers, and filesystem shares. TypeFerence needs to
consume those systems without becoming another registry server or allowing feed
state to leak into deterministic compilation.

## Decision

Add a phase before build:

`restore source packages → build unlinked targets → link deployment → publish/run`

The project manifest declares exact package dependencies. `typeference pack`
creates a canonical content-addressed `.tferpkg`. `typeference restore` resolves
the complete graph through externally configured namespace routes, verifies
digests, writes a canonical committed `typeference.lock`, and materializes the
locked tree under `obj/typeference/packages`.

The lock records identity, exact version, digest, exports, and dependency edges.
It never records a feed URL, credential, machine path, or retrieval timestamp.
Build is offline and frozen: it reads a supplied materialized package directory,
re-verifies every locked digest, and never searches feeds.

Initial providers are filesystem and generic HTTP. JFrog Generic is the HTTP
layout with JFrog authentication support. Azure Artifacts uses an official
artifact transport selected by the implementation. Feed routing is scoped and
deterministic: one namespace maps to one feed, and no fallback search is allowed.

Version 4 uses exact versions and rejects graph conflicts and cycles. Range
solving is deferred.

## Consequences

- An enterprise can publish defaults to existing artifact infrastructure and a
  consumer can restore the complete dependency tree with one command.
- Builds remain reproducible and work without network access.
- Changing staging/prod feed addresses does not change source identity.
- The package client is intentionally small; TypeFerence does not host registries.
- A global content-addressed cache may accelerate restore, while the build-facing
  materialized tree remains project-local and inspectable.

## Alternatives considered

- **Resolve directly during build.** Makes builds network-dependent and feed-state
  sensitive. Rejected.
- **Commit vendored dependency trees.** Reproducible but operationally hostile and
  duplicates package bytes. Supported as an external workflow, not the model.
- **Implement a semantic-version solver immediately.** Adds complexity before the
  identity and feed boundary are proven. Deferred.
