# 0023 — Define the `.tfer` source format

**Status:** Accepted (2026-07-30)

## Context

ADRs 0012 and 0013 anticipated a frontmatter-plus-body source format but deferred
its serialization rules. ADR 0020 then closed the version 4 language, and the
implementation began accepting `.tfer` files, without a normative decision that
defined their fences, bodied resource kinds, parsing boundary, or canonical text
semantics.

Those choices are semantic. A body becomes compiled instructions or context and
therefore affects target bytes. Leaving the format implicit would make the Go
loader, rather than the specification, the source of truth.

## Decision

A `.tfer` document is one UTF-8 resource with:

1. an exact opening `---` fence line;
2. one YAML mapping between that line and the next exact `---` fence line; and
3. the remaining text as its body.

Input is BOM-stripped and CRLF-normalized to LF before the split. Fence markers
must occupy a line by themselves. The YAML head is parsed as exactly one document
using the same closed version 4 field rules as a `.yaml` resource. YAML implicit
scalar typing does not determine native context values; those scalar tokens are
interpreted against the declared TypeFerence type.

Only two resource kinds may carry a non-whitespace body:

- a unimodal `skill`, whose body is its `instructions`; and
- a `context`, whose body is the text value governed by its `contextType`.

A skill may put instructions in either frontmatter or the body, never both. A
multimodal skill has no single instruction body: every variant keeps its
instructions in the `variants` mapping, and any non-whitespace body is an error.
All other kinds reject a non-whitespace body.

The body is otherwise preserved after the input normalization above. Context-body
requiredness is checked after trimming only for the presence test; emitted content
retains the normalized authored text.

Plain `.yaml` resources remain head-only YAML documents and cannot carry a
TypeFerence body.

## Consequences

- `.tfer` source has one portable, testable grammar rather than an
  implementation-private convention.
- Unimodal prose remains pleasant to author while multimodal instructions remain
  structurally separated by mode.
- Body text participates in source-package and target identity through the normal
  canonical source rules.
- The deferred format language in ADRs 0012 and 0013 is superseded by this ADR.

## Alternatives considered

- **Infer frontmatter without fences.** Ambiguous with ordinary YAML and markdown;
  rejected.
- **Allow a default body on multimodal skills.** Creates two competing default-mode
  mechanisms and obscures which requirements apply; rejected.
- **Permit bodies on every kind.** Accepts text with no defined semantic effect and
  violates the closed-language rule; rejected.
