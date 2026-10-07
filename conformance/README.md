# TypeFerence determinism suite

Language-neutral fixtures the compiler must reproduce as byte-identical
artifacts. The version 7 corpus pins the specification's canonical output:
the Go implementation runs it in CI, and a digest mismatch on any success
fixture is a broken build. Error fixtures must fail with a diagnostic; the
diagnostic text is not part of the contract (ADR-0003).

## Layout

```
conformance/fixtures/<NNN-name>/
  manifest.json      what to build and what to expect
  source/            the TypeFerence source tree for the fixture
  packages/          (optional) dependency package sources
```

`manifest.json` fields:

| Field | Meaning |
| --- | --- |
| `description` | What the fixture exercises. |
| `expect` | `success` or `error`. |
| `packages` | Optional directory of dependency package sources. The runner stages each into a temporary filesystem feed (packing in dependency order, restoring each package that has dependencies) and restores a copy of `source/` against it before building. A restore failure counts as the fixture's result. |
| `digests` | For `success` fixtures: the expected `typeference-directory-v1` digest of the emitted `agent-plugin` directory. |

Fixtures `001`-`099` succeed; `101` onward fail.

## Regenerating digests

Digests are produced by the implementation, never typed by hand:

```sh
cd go && go test ./conformance -update
```

Regenerate only together with the specification change and ADR that justify
the new bytes, and review the resulting artifact diff.
