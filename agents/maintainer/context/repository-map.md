# Repository map

| Path | Role |
| --- | --- |
| `docs/specification.md` | Normative specification (source of truth). |
| `docs/whitepaper.md` | Motivation and design narrative. |
| `docs/decisions/` | Architecture decision records. |
| `src/` | C# reference implementation (.NET 10). |
| `go/` | Go implementation (static binary; module `github.com/buchk/TypeFerence/go`). |
| `tests/` | C# test suite, including the conformance runner. |
| `go/conformance/` | Go conformance runner (`-update` regenerates digests). |
| `conformance/` | Shared cross-implementation fixture corpus. |
| `examples/helio` | Example organization used by tests and the quick start. |
| `dist/` | Committed reference output of `examples/helio` (byte-compared in CI). |
| `agents/maintainer/` | This definition; compiled into the root `AGENTS.md` and `dist/maintainer/`. |
| `.github/workflows/ci.yml` | Build, test, conformance, and self-host drift gates. |

The root `AGENTS.md` is generated from this definition. To change it, edit the
resources under `agents/maintainer/` and run `make selfhost`; never edit the
generated file directly.
