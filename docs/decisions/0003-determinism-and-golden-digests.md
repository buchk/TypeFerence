# 0003 — Deterministic output, enforced by golden digests

## Status

Accepted (2026-10-07).

## Context

A marketplace is reviewed as a diff. If identical source can produce
different bytes, every build is a diff nobody can trust, provenance stops
meaning anything, and a Windows checkout disagrees with CI.

## Decision

1. **Identical source compiles to identical bytes on every platform.**
   Canonicalization is defined in the specification: UTF-8 without BOM, LF,
   forward-slash paths in code-point order, per-artifact JSON member order,
   sorted keys for map-like objects, authored JSON number lexemes preserved.
   JSON strings use the escape policy inherited from the original .NET
   serializer (`"`, `&`, `'`, `+`, `<`, `>`, backtick, backslash, controls,
   and all non-ASCII as uppercase `\uXXXX`), pinned by `go/internal/jsonx`
   tests. Changing it would change every digest for no semantic gain.
2. **Digests are portable.** `typeference-directory-v1` hashes normalized
   text for files that are valid UTF-8 and exact bytes for anything else, so
   line-ending conversion cannot change identity and binary files are never
   normalized away. Destinations that differ only in letter case, or that
   spell a shared directory differently, are errors on every filesystem.
3. **The contract is the fixture corpus.** Each success fixture under
   `conformance/fixtures/` records the digest of its emitted target; each
   error fixture records only that build fails, so diagnostics can improve
   freely. `conformance/.gitattributes` keeps fixture bytes (CRLF, BOM,
   binary) from being converted by git. Digests are regenerated with
   `go test ./conformance -update`, never typed, and only in the change that
   alters the specification.
4. **Committed reference outputs.** `dist/` is the Helio marketplace's build
   and `dist-maintainer/` plus the root `AGENTS.md` are the maintainer
   definition's; tests and CI fail on any drift. CI also validates every
   emitted file against the host schemas (`docs/output-contract.md`).
5. **Source identity is an explicit resource set.** The source digest hashes
   the manifest, the lockfile, and the closure of referenced documents and
   files, never a directory walk, so output, caches, and unreferenced files
   cannot change it.

## Consequences

- Unexplained digest churn is visible in review, and a regenerated digest
  always arrives with the specification text that explains it.
- Every canonicalization or composition ruling ships with a fixture.

## Alternatives considered

- **Compare diagnostics byte for byte.** It would freeze error wording.
- **Commit every fixture's full output tree.** Digests are compact; `dist/`
  already provides one fully materialized reference.
- **A new digest name when binary-safe hashing arrived.** Every earlier
  artifact was UTF-8, so no digest changed; a new name would add ceremony and
  protect nothing.
