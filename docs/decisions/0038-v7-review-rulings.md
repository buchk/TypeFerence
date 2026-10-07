# 0038 — Version 7 review rulings: portable bytes, requirements, provenance, and inherited bindings

## Status

Proposed (2026-10-07). Amends the version 7 specification ("Source
membership and digests", "Field tables", "Composition", "Instances", "Skill
files", "Diff and security"). Responds to the review of
`feat/v7-plugin-authoring`.

## Context

The review found six cases where the version 7 implementation did not
deliver what the specification promised. In each case the existing fixtures
didn't cover the situation:

1. Skill-file destinations differing only in letter case (`references/A.txt`
   and `references/a.txt`) were accepted. A case-insensitive filesystem kept
   one file holding the other's bytes, so output differed by platform.
2. `typeference-directory-v1` and `diff` normalized every file as text. Once
   skills could ship binary files, removing a binary's leading `EF BB BF`
   bytes, or rewriting its `0D 0A`, went undetected.
3. A local abstract requirement (`required: true`, no skill) replaced an
   implementation inherited from an embedded profile with none, so a valid
   composition failed.
4. Agent descriptions were not re-checked after field references were
   resolved. A multiline text value could inject a frontmatter fence.
5. Provenance kept only the first contributor of a document held by several
   layers, and of identical bindings that converged.
6. The playground form wrote multiline values unquoted and read block
   scalars with its own incomplete parser.

The review also asked for an explicit ruling: what happens to an embedded
agent's `with` bindings?

## Decision

1. **Portable destinations.** Two destinations in one skill directory
   collide when they are equal after lowercasing. Collisions are errors,
   whichever filesystem builds.
2. **Binary-safe digests and diffs.** `typeference-directory-v1` hashes
   normalized text for a file whose bytes are valid UTF-8 and the exact bytes
   of any other file. `diff` compares the same way. Every artifact the
   compiler emitted before skill files existed is valid UTF-8, so existing
   digests do not change.
3. **Requirements accumulate; they never select.** A capability is required
   when any layer at any depth requires it. An abstract requirement adds that
   obligation without selecting, replacing, or erasing an implementation.
   Implementation selection is unchanged: shallowest wins, identical
   implementations converge, different ones at one depth are ambiguous.
4. **Descriptions are validated after rendering.** Agent descriptions, like
   skill descriptions, must remain single lines without control characters
   after field references are resolved.
5. **Provenance keeps every contributor.** Deduplication applies to emitted
   content only. Provenance records each contributor of a document held by
   several layers, each contributor of converged identical bindings, and each
   source of a parameter binding.
6. **Embedded agents' bindings: shallowest wins.** An agent's bindings are
   its own `with` plus those promoted from the agents it embeds, using the
   same rule as capability bindings. Embedding an agent and binding a name
   re-points the embedded agent at other data, which is what makes agent
   embedding useful for templates. Embedding it without binding keeps its
   data. Every objective renders with the embedding agent's bindings. The
   unused-binding check applies to an agent's own `with`.
7. **The playground form uses the compiler's reading.** The WebAssembly
   bridge returns every data document's values as the compiler parsed them.
   The form writes any value that is not a safe plain scalar as a
   double-quoted string with JSON escapes, which the grammar accepts.

## Consequences

- Output is identical on case-sensitive and case-insensitive filesystems.
- Committed binary artifacts are verified byte for byte, while committed text
  still survives a Windows checkout's line-ending conversion.
- Profiles can state requirements that embedding layers satisfy, and agents
  can restate requirements their profiles already meet.
- A frontmatter injection path through bound data is closed.
- Bundles answer "where did this come from" completely.
- Agent embedding becomes a specialization mechanism for templates: "this
  agent, for my team."

## Alternatives considered

- **Freeze an embedded agent's bindings.** It preserves the embedded agent's
  exact meaning, but makes embedding an agent nearly useless for templates
  and is inconsistent with every other composition rule.
- **A new digest name (`typeference-directory-v2`).** No existing digest
  changes, because every previously emitted file is UTF-8, so a new name adds
  ceremony without protecting anything.
- **Reject non-ASCII destination names instead of folding case.** It is more
  restrictive than needed. Lowercasing covers the collision cases that occur
  in practice.
