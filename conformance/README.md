# TypeFerence determinism suite

Language-neutral fixtures the compiler must reproduce as byte-identical
artifacts. The current version 4 corpus pins the specification's canonical
output: the Go implementation runs it in CI, and a digest mismatch on any current
fixture is a broken build.

Fixtures explicitly marked `"language": "legacy-v3"` form a separate archival
golden regression corpus. They preserve retired bytes through a test-only compiler
switch but do not describe source accepted by the CLI, LSP, package, or playground
entrypoints. This keeps historical determinism evidence without calling retired
version 3 behavior current conformance. The corpus once compared Go and C#; after
ADR-0014 it is a single-implementation golden guarantee.

## Layout

```
conformance/fixtures/<NNN-name>/
  manifest.json      what to build and what to expect
  source/            the TypeFerence source tree for the fixture
  signatures.json    (only for signing fixtures; deliberately outside source/)
```

`manifest.json` fields:

| Field | Meaning |
| --- | --- |
| `description` | What the fixture exercises. |
| `language` | Omitted for current v4; `legacy-v3` only for the archival corpus. |
| `expect` | `success` or `error`. Error fixtures must fail compilation in every implementation; the diagnostic text is not part of the contract (see ADR-0005). |
| `emitArd` | Optional ARD publisher domain; presence enables `--emit-ard`. |
| `trustSignatures` | Optional signature map path relative to the fixture directory. |
| `allowUnsignedTrust` | Optional; enables the unsigned-staging escape hatch. |
| `digests` | For `success` fixtures: the expected `typeference-directory-v1` digest of each emitted top-level target directory (`neutral`, `codex`, `copilot`, `cursor`, `ard`). |

All fixtures build with `--target all`. `TestConformance` runs current v4;
`TestLegacyV3Golden` runs the archival corpus.

The [specification evidence matrix](../docs/conformance-matrix.md) records which
normative areas are covered by v4 golden fixtures and which are enforced by
focused unit tests. Canonicalization and composition rulings require a current
fixture in the same change; unit tests are supporting evidence, not a substitute
for the golden-byte contract in those areas.

## Running

`cd go && go test ./conformance` (or `make conformance`).

## Updating expectations

```
cd go && go test ./conformance -run TestConformance -update
```

Never hand-edit a digest. If a digest changes, either the spec changed (requires an
ADR) or the compiler broke.

## Byte fidelity

`conformance/.gitattributes` disables git newline conversion beneath `fixtures/`:
several fixtures pin CRLF, missing-trailing-newline, and BOM handling, and would be
destroyed by autocrlf.
