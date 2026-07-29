# 0020 — Close the source language and preserve modes

**Status:** Accepted (2026-07-29)

## Context

ADRs 0012, 0013, 0016, 0017, and 0019 introduced invocation variants, typed
context, abstract requirements, tools, and two context lifecycles. Their first
implementation deliberately accepted several incomplete representations:

- context types embedded a small, silently partial JSON Schema evaluator;
- `contextFiles` remained a raw-path escape hatch;
- resolved context values were not fully emitted;
- variant requirements were flattened across modes;
- tool validation proved declaration existence but did not preserve a runtime
  import for later fulfillment;
- a sealed abstract requirement was accepted even though no implementation
  existed to protect.

Those compromises prevent the type system from making a closed-world claim. They
also make unsupported language appear accepted even when it has no consequence.

## Decision

Typed resources advance to `schemaVersion: 4`, a closed version that:

1. replaces embedded JSON Schema for context sources with a small native type
   language (`string`, `text`, scalar numerics/boolean, lists, maps, and named
   context types);
2. makes context instances first-class values with fully emitted fields and body;
3. removes `contextFiles`; raw prose is an explicitly declared text-body context;
4. preserves base and per-mode context/tool requirements rather than unioning
   every mode;
5. treats tools as runtime imports that build declares and link fulfills;
6. rejects sealed bindings without a concrete implementation;
7. distinguishes an omitted allow-list from an explicit empty allow-list so a
   disjoint intersection cannot fail open.

Unknown fields, schema constructors, and unsupported schema versions are errors.
JSON Schema remains a generated interoperability projection, not the context
source language.

## Consequences

- Version 4 can honestly claim that accepted context and tool declarations have
  observable compile/link consequences.
- Version 3 remains useful historical input but is not silently interpreted as
  version 4; migration is explicit.
- Target adapters must carry mode requirements and complete context values.
- Authors wanting an opaque file must name that choice with a raw-text context
  type.
- This ADR supersedes the deferred/compatibility portions of ADRs 0012, 0013,
  0016, 0017, and 0019; their core object-model decisions remain accepted.

## Alternatives considered

- **Implement full JSON Schema.** Too large and semantically open for a language
  whose purpose is predictable composition. Rejected for the source language.
- **Keep `contextFiles` as an escape hatch.** It would keep a path-shaped hole in
  every context policy. Rejected.
- **Union all variant requirements.** Makes an optional mode constrain unrelated
  targets and erases why variants exist. Rejected.
