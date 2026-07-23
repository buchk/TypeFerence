# TypeFerence determinism suite

Language-neutral fixtures the compiler must reproduce as byte-identical
artifacts. This suite pins the specification's canonical output: the Go
implementation runs it in CI, and a digest mismatch on any fixture is a broken
build. The fixtures and canonicalization rulings (ADR-0004) are the same corpus
that once verified two implementations against each other; a single
implementation now carries it forward as a golden-file guarantee (ADR-0014).

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
| `expect` | `success` or `error`. Error fixtures must fail compilation in every implementation; the diagnostic text is not part of the contract (see ADR-0005). |
| `emitArd` | Optional ARD publisher domain; presence enables `--emit-ard`. |
| `trustSignatures` | Optional signature map path relative to the fixture directory. |
| `allowUnsignedTrust` | Optional; enables the unsigned-staging escape hatch. |
| `digests` | For `success` fixtures: the expected `typeference-directory-v1` digest of each emitted top-level target directory (`neutral`, `codex`, `copilot`, `cursor`, `ard`). |

All fixtures build with `--target all`.

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
