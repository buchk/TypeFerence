# 0005 — A closed `.tfer` grammar with schema-directed scalars

## Status

Accepted (2026-10-07).

## Context

Source documents are Markdown with structured frontmatter. Parsing that
frontmatter as YAML means accepting whatever a YAML library accepts, and
letting spelling decide type: `no`, `on`, `1.0`, and `~` change meaning
depending on the resolver. Two layers then decide what a document means.

## Decision

1. **One source format.** A `.tfer` document is an exact `---` fence, a
   frontmatter mapping, a closing fence, and a Markdown body. Its kind comes
   from its suffix and its identity from the package name plus its path;
   documents carry no `kind`, `id`, or schema version. Only the manifest
   declares `schemaVersion`.
2. **A closed grammar, parsed by `tferlex` and nothing else.** Indentation
   nests; tabs in structure are errors; literal block scalars only; `[]` and
   `{}` are the only flow tokens; anchors, aliases, tags, quoted keys, and
   multiple documents do not exist. Double-quoted strings use JSON escapes.
   Every field table is closed: unknown fields are errors.
3. **Schema-directed scalars.** The grammar never types an unquoted scalar;
   the receiving field does. A string field keeps the text verbatim, a boolean
   field accepts only unquoted `true` or `false`, and an integer field only a
   signed 64-bit integer token. A quoted scalar is always a string. There is
   no floating-point type.

## Consequences

- Sources read the way people write them. The only quoting rule is to quote a
  value that must be a string in a field whose type is not string.
- The specification describes the parser that runs, and diagnostics name the
  file and line.
- YAML is still used to read foreign formats in `import`.

## Alternatives considered

- **YAML 1.2 core schema.** It types `true`, `null`, `1e3`, and `.inf` by
  spelling, which is the ambiguity this closes.
- **A YAML library with stricter post-checks.** The accepted surface would
  still be whatever the library accepts.
- **Syntactic typing (quote every string).** It costs authors on every
  description and path and buys nothing a declared field type does not.
