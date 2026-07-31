# 0025 — Require linked-output ownership before reset

**Status:** Accepted (2026-07-31)

## Context

`typeference link --out` originally removed the resolved output path recursively
after checking only that it did not overlap the unlinked input. That made relink
convenient, but an accidental unrelated path such as an examples directory could
be deleted without evidence that TypeFerence had created it.

Link must remain repeatable while failing closed around user-owned files. The
linker already writes root provenance only after materialization completes, so it
has a natural ownership record that does not affect any linked agent digest.

## Decision

Link may materialize into an absent or empty output directory. It may recursively
replace a non-empty directory only when the directory root contains a valid
schema version 1 `.typeference/link-provenance.json` produced by a completed link.
All other existing non-empty outputs are rejected before deletion or writing.
Files, filesystem roots, and paths that overlap the unlinked input are also
rejected.

The provenance record is evidence that TypeFerence owns the output layout for
replacement; it is not an authorization mechanism or cryptographic trust proof.
Its structure must parse as the closed current link-provenance schema before it
authorizes a reset.

## Consequences

- A typo in `--out` cannot recursively delete an arbitrary non-empty directory.
- Repeating link against a prior completed output still removes stale generated
  files and produces a clean materialization.
- A partial or manually damaged linked directory must be emptied or removed by
  the operator before retrying; malformed provenance does not authorize cleanup.
- Linked-agent digests remain unchanged because the ownership record stays at the
  linked-output root, outside each agent directory.

## Alternatives considered

- **Always reject non-empty output.** Safest mechanically, but makes ordinary
  deterministic relinking needlessly manual. Rejected.
- **Accept a force flag for arbitrary deletion.** Turns a path mistake into a
  destructive operation and gives the linker more authority than it needs.
  Rejected.
- **Treat the marker's presence alone as ownership.** A malformed or unrelated
  file with the same name would authorize deletion. Rejected in favor of parsing
  the closed provenance envelope.
