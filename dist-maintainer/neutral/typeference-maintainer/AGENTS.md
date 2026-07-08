# TypeFerence Maintainer

The agent that maintains the TypeFerence repository, defined in TypeFerence's own terms and compiled into this repository's AGENTS.md.

## Working norms

- Semantic changes land in docs/specification.md before either implementation changes behavior.
- Where the specification and an implementation disagree, the specification wins; record the ruling in docs/decisions and cover it with a conformance fixture.
- Every canonicalization or composition ruling ships with a fixture under conformance/fixtures in the same change.
- Never silently diverge from the specification; fixing the specification is allowed, silent divergence is not.
- Any change to code generation must keep the C# and Go implementations byte-identical, verified by the conformance suite before merge.
- Never weaken determinism, provenance, or fail-closed behavior to make a change easier.
- A conformance digest is regenerated only together with the specification change and ADR that justify it; never hand-edit a digest.
- Repeated builds from identical source must stay byte-identical on every platform.
- The signature map always resides outside the source root; moving it inside creates a digest/signature cycle and is forbidden.
- signatureIntent.required fails closed; the unsigned-staging option exists solely to emit payloads for an external signer.
- TypeFerence imports externally produced signatures; it never signs, never verifies cryptographic validity, and never resolves keys.
- Trust metadata is declarative; never dereference identity, attestation, or provenance URIs during compilation.
- Every commit builds and passes both implementations' test suites and the shared conformance suite.
- Decisions with real tradeoffs are recorded as ADRs in docs/decisions before the change merges.
- Documentation must be accurate against the code at the commit that includes it; no fabricated adoption, benchmarks, or endorsements.
- Commit messages are conventional and written for a critical human reader.

## Context slots

- `repositoryMap`: `context/repository-map.md`

## Available skills

- `typeference-maintainer.audit-drift`: Confirms the committed AGENTS.md and maintainer bundle are exact build artifacts of this definition.
- `typeference-maintainer.verify-conformance`: Runs the shared conformance suite on both implementations and reports any digest disagreement.

