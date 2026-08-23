# 0026 — v5: one closed frontmatter grammar and syntactic scalar typing

## Status

Accepted. Amends the specification to `schemaVersion: 5` for typed resources.
Extends ADR-0023 (which introduced `.tfer` as a second format alongside YAML)
and supersedes its dual-format acceptance.

## Context

Version 4 accepted two surface formats: bare YAML documents and
frontmatter-plus-body `.tfer`. This forced every guarantee to be negotiated
with a legacy parser:

- YAML's implicit resolver guesses scalar types from syntax, so "is `3` an
  integer or a string" had to be settled by schema-directed interpretation on
  top of whatever the resolver produced. Two layers decided what a value meant.
- YAML's grammar is larger than the language needs (anchors, aliases, tags,
  flow collections, multi-document streams), each a potential divergence
  between implementations and a hole in canonicalization.
- The project's thesis — quoted is a string, digits are an integer, structure
  is declared — was enforced nowhere at the syntax level.

Separately, the numeric vocabulary inherited YAML's split (`integer`,
`number`). A floating-point type is hostile to the determinism guarantee:
IEEE 754 has multiple NaN payloads, negative zero, and shortest-round-trip
formatting that varies across runtimes; canonicalizing floats into stable
digest bytes is a known swamp. The js/wasm playground round-trips values
through JSON doubles, compounding the risk.

Go's native numeric shelf was considered as the type vocabulary and rejected:
`int` is implementation-sized, which contradicts byte-identical output across
platforms; Go lacks enums, sum types, and refinements that later amendments
need; and Go is the reference realization, not the definition — the spec must
not become a description of one implementation's accidents.

## Decision

Version 5 accepts exactly one source format and defines its own closed
surface grammar:

1. **One format.** `.tfer` frontmatter-plus-body documents are the sole
   source. Bare YAML documents are rejected with a format-naming diagnostic.
   Manifests and trust configuration use the same grammar under their existing
   names (`typeference.tfer`, `typeference.trust.tfer`).
2. **A closed grammar, not YAML.** Two-space indentation nesting, flow-free
   sequences, quoted strings, block scalars (`|`, `-|`, `|2`) whose chomping
   and relative-indentation semantics are cited from YAML 1.2 rather than
   reinvented, inert comments, duplicate keys are errors. Anchors, aliases,
   tags, flow collections, and multi-doc streams do not exist and are parse
   errors if attempted.
3. **Syntactic scalar typing, no inference.** Quoted = string. Unquoted digit
   runs = integers, arbitrary precision, canonicalized as the written digit
   string. Decimal lexemes are decimals preserved verbatim (`0.50` ≠ `0.5`;
   they are distinct bytes). `true`, `false`, `null` are reserved words. Every
   other bare word is an error — nothing is ever guessed.
4. **No floating-point type.** The `number` constructor does not exist in
   version 5. Schemas declare `integer` or `decimal`. Floating-point math is
   programming, not organizational-behavior declaration; a future amendment
   may add it as a new constructor without breaking anything.

Rejected alternatives:

- **Keep dual formats** (status quo): preserves two parsers, two error
  vocabularies, and schema-directed value interpretation forever.
- **Adopt Go's standard types**: non-portable `int`; no path to enums or
  refinements; spec-first inversion.
- **Keep a float/`number` constructor with fixed formatting rules**: even
  carefully specified float canonicalization leaves digest stability hostage
  to serialization edge cases, and no organizational policy needs arithmetic.
- **Invent new block-scalar semantics instead of citing YAML 1.2**: gratuitous
  divergence from the one thing YAML got right and users already know.

## Consequences

- One lexer/parser serves resources, manifests, trust files, and (later) any
  tooling input; error messages come from one vocabulary.
- Canonicalization becomes purely lexical: digests depend on written bytes,
  never on resolver behavior. The determinism suite can pin decimal lexemes
  and integer digit strings directly.
- Every v4 fixture converts mechanically; conversion is syntax-only quoting.
- Authors coming from YAML keep indentation, comments, and block scalars;
  they lose only the parts of YAML that made meaning ambiguous.

The companion decision on behavioral prose is recorded in ADR-0027.
