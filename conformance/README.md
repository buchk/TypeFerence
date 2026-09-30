# TypeFerence determinism suite

Language-neutral fixtures the compiler must reproduce as byte-identical
artifacts. The current version 6 corpus pins the specification's canonical
output: the Go implementation runs it in CI, and a digest mismatch on any current
fixture is a broken build.

Fixtures marked `"language": "legacy-v5"` or `"language": "legacy-v3"` form
separate archival golden regression corpora. They preserve retired bytes
through archival loaders that only the conformance runner selects; they do not
describe source accepted by the CLI, language server, package, or playground
entry points. This keeps historical determinism evidence without calling
retired behavior current conformance. The corpus once compared Go and C#; after
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
| `language` | Omitted for the current version 6 corpus; `legacy-v5` or `legacy-v3` for the archival corpora. |
| `expect` | `success` or `error`. Error fixtures must fail compilation; the diagnostic text is not part of the contract (see ADR-0005). |
| `emitArd` | Optional ARD publisher domain; presence enables `--emit-ard`. |
| `trustSignatures` | Optional signature map path relative to the fixture directory. |
| `allowUnsignedTrust` | Optional; enables the unsigned-staging escape hatch. |
| `generatorVersion` | Setup wizard fixtures only: the generator that produced the source (ADR-0028). |
| `digests` | For `success` fixtures: the expected `typeference-directory-v1` digest of each emitted top-level directory (`agent-plugin`, `neutral`, `ard`), and `scaffold` for wizard fixtures. |

Current fixtures build `neutral` and `agent-plugin`; archival fixtures build
`neutral` only, the one target of theirs that version 6 still defines.
`TestConformance` runs the current corpus, and `TestLegacyV5Golden` and
`TestLegacyV3Golden` run the archival ones.

Fixtures 058–074 are version 6 successes and 080–099 version 6 errors. Every
fixture numbered below 058 is archival.

The [specification evidence matrix](../docs/conformance-matrix.md) records which
normative areas are covered by golden fixtures and which are enforced by
focused unit tests. Canonicalization and composition rulings require a current
fixture in the same change; unit tests are supporting evidence, not a substitute
for the golden-byte contract in those areas.

## Running

`cd go && go test ./conformance` (or `make conformance`).

## Updating expectations

```
cd go && go test ./conformance -update
```

Never hand-edit a digest. If a digest changes, either the spec changed (requires an
ADR) or the compiler broke. After updating, confirm that only the fixtures the
specification change justifies were rewritten.

## Byte fidelity

`conformance/.gitattributes` disables git newline conversion beneath `fixtures/`:
several fixtures pin CRLF, missing-trailing-newline, and BOM handling, and would be
destroyed by autocrlf.
