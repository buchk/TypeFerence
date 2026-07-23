# Repository map

| Path | Role |
| --- | --- |
| `docs/specification.md` | Normative specification (source of truth). |
| `docs/whitepaper.md` | Motivation and design narrative. |
| `docs/decisions/` | Architecture decision records. |
| `go/` | The implementation (static binary; module `github.com/buchk/TypeFerence/go`). |
| `go/cmd/typeference-lsp/` | Language server for `.tfer`/`.yaml` authoring. |
| `editors/vscode/` | VS Code client for the language server. |
| `go/conformance/` | Determinism runner (`-update` regenerates digests). |
| `conformance/` | Golden-file fixture corpus. |
| `examples/helio` | Example organization used by tests and the quick start. |
| `dist/` | Committed reference output of `examples/helio` (byte-compared in CI). |
| `agents/maintainer/` | This definition; compiled into the root `AGENTS.md` and `dist-maintainer/`. |
| `.github/workflows/ci.yml` | Build, test, determinism, and self-host drift gates. |

The root `AGENTS.md` is generated from this definition. To change it, edit the
resources under `agents/maintainer/` and run `make selfhost`; never edit the
generated file directly.
