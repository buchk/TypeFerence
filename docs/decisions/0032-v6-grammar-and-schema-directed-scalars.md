# 0032 — v6: the closed grammar as the only parser, and schema-directed scalars

## Status

Proposed (2026-09-29). On acceptance: amends ADR-0026 decisions 2 and 3 for
version 6 sources. Companion to ADR-0030, whose path-derived identity removes
most of the scalars ADR-0026 was written to type.

## Context

ADR-0026 specified a closed frontmatter grammar with syntactic scalar typing:
quoted is a string, digit runs are integers, `true`/`false`/`null` are reserved,
and every other unquoted word is an error. Two facts about version 5 as built:

- **The grammar was not the parser.** A dedicated `tferlex` package existed,
  but the resource loader and manifest parser read frontmatter with a YAML
  library and post-checked the result. YAML's resolver, not the specified
  grammar, decided what was accepted.
- **Syntactic typing was never enforced, and nobody wrote to it.** Every
  version 5 source, including `examples/helio`, wrote unquoted text such as
  `displayName: Helio Payments Repository Agent`, which the specification
  called an error. The specification and the implementation disagreed, and
  the corpus followed the implementation.

Enforcing ADR-0026 as written would make every description, display name, and
path quoted. Version 6 adds required plain-language descriptions to agents,
skills, and plugins and makes references paths, so the quoting burden would
land on exactly the fields contributors write most. The rule's purpose, that no
resolver ever guesses a type from spelling, does not need it: every field
already declares its type.

The integer rule has a similar gap. ADR-0026 says integers are arbitrary
precision; the implementation has always parsed them as signed 64-bit values.

## Decision

1. **The closed grammar is the only parser for source documents.** Version 6
   manifests and resource documents are parsed by the TypeFerence grammar
   (`tferlex`) and nothing else. A YAML library remains for the archival
   languages, for files outside the source language (deployment and feed
   configuration, eval scenarios), and for reading foreign formats in
   `typeference import` (ADR-0033). The trust configuration keeps its closed,
   YAML-based reader for now; moving it onto the grammar is follow-up work,
   not part of this decision. Grammar and field diagnostics name their file
   and, where one exists, the line.
2. **Schema-directed scalar typing.** The grammar returns unquoted scalars as
   plain text and never assigns a type. The receiving field declares one: a
   string field keeps the text verbatim (so `no`, `true`, and `1.0` are strings
   there); a boolean field accepts only unquoted `true` or `false`; integer and
   decimal context fields accept only unquoted numeric tokens. A quoted or
   block scalar is always a string and never satisfies a boolean, integer, or
   decimal field. Type still never comes from spelling; it comes from the
   schema, which is one layer.
3. **Integers are JSON integer tokens in the signed 64-bit range**, preserved
   as written. This specifies what the implementation has always done;
   arbitrary precision can be added later without breaking any valid source.
   Decimals are JSON number tokens preserved verbatim, as in ADR-0026, and
   there is still no `number` type; declaring one is an error that names
   `integer` and `decimal`.
4. **Grammar surface.** Indentation nests blocks (any deeper indentation,
   consistent within a block, rather than exactly two spaces); tabs in
   structural indentation are errors. Literal block scalars take `|`, `|-`,
   `|N`, and `|N-`; folded (`>`) and keep (`|+`) chomping do not exist.
   Empty values, `null`, and `~` are null. `[]` and `{}` are the only flow
   tokens, as empty collections; every other flow collection, anchors,
   aliases, tags, quoted keys, and multiple documents are errors.
   Double-quoted strings use JSON escapes. A `#` begins a comment only at the
   start of content or after whitespace, outside quoted and block scalars.

## Consequences

- Sources read the way people already wrote them. The one quoting rule left
  is the honest one: quote a value only when it must be a string in a field
  whose type is not string.
- The specification now describes the parser that runs; the conformance
  corpus pins it (`066-schema-directed-scalars`, `089-quoted-boolean-error`,
  `090-flow-collection-error`, `094-number-type-error`,
  `098-integer-range-error`).
- ADR-0026's other rulings stand: one format, no floating point, decimal
  lexemes preserved, no implicit resolution.

## Alternatives considered

- **Enforce ADR-0026 as written.** Rejected: quoting every description and
  path is the authoring cost version 6 exists to remove, and the rule buys
  nothing a declared field type does not already provide.
- **Adopt YAML 1.2's core schema.** Rejected: its resolver types `true`,
  `null`, `~`, `1e3`, and `.inf` by spelling, the ambiguity ADR-0026 closed.
- **Keep a YAML library as the parser and tighten its post-checks.** Rejected:
  two layers would again decide what a document means, and the accepted
  surface would be whatever the library accepts.
- **Arbitrary-precision integers now.** Deferred: no organizational value
  needs them, and the change is additive when one does.
