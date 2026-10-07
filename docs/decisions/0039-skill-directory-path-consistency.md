# 0039 — Consistent paths throughout a skill directory

## Status

Proposed (2026-10-06). Amends the specification's "Skill files" rules and
extends ADR-0038's portable-destination ruling.

## Context

Comparing lowercase complete destinations catches `A.txt` versus `a.txt`,
but misses `references/A/one.txt` versus `references/a/two.txt`. Windows
merges those directories under the first spelling; a case-sensitive filesystem
keeps both. The emitted paths and directory digests consequently differ.
Similarly, a file used as another destination's directory must be diagnosed
before the writer starts emitting artifacts.

## Decision

Validate each destination and every directory prefix in the complete skill
directory. Lowercase paths identify claims, but retain each claim's authored
spelling and whether it is a file or directory:

- Files cannot claim an already claimed path.
- Directories can share a claim only with exactly the same spelling.
- A file and directory cannot claim the same lowercase path.

Use the same validation for flattened skill files and the complete emitted
directory, where generated documents and schemas also claim paths. Validation
occurs during planning, before build resets or writes its output. Accepted
paths and their bytes do not change.

## Consequences

Case aliases at any directory depth fail on every platform. Consistently
spelled mixed-case directories remain valid. Existing success fixture digests
and reference artifacts remain unchanged. Fixtures 130 and 131 cover directory
case aliases and file/directory conflicts; focused tests cover input order,
extension, generated files, and preservation of existing output on failure.

## Alternatives considered

- **Lowercase every emitted path.** This changes authored paths and can break
  links or commands in instructions and scripts.
- **Reject all uppercase paths.** Unnecessarily rejects consistent layouts
  and imported resources that already work on every supported platform.
- **Let the filesystem reject conflicts.** Case-insensitive filesystems can
  silently merge directories, and write-time errors can leave partial output.
