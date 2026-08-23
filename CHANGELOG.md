# Changelog

All notable changes to TypeFerence are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow semantic
versioning. Tool versions (this file) are independent of the typed-resource
`schemaVersion` (currently 5) and the trust configuration `schemaVersion`
(also 5), which only change when the source formats change incompatibly.

TypeFerence is an experimental reference implementation; pre-1.0 versions make no
compatibility promises between minor versions.

## [Unreleased]

### Added

- **Deterministic setup wizard** ([ADR-0028](docs/decisions/0028-deterministic-setup-wizard.md)).
  `typeference init --answers answers.json [--out DIR] [--verify sha256:...]`
  scaffolds a complete multilevel v5 suite — typed norm contexts, a profile
  embedding chain, one concrete agent — from a strict, versioned answer set.
  The browser playground's new Setup wizard runs the same generator over the
  wasm bridge and offers the identical digest, so the local checkout can be
  verified byte-for-byte against the browser session. The canonical answer
  set's golden fixture (057) compiles through the ordinary compiler and pins
  generator × schema to bytes in CI.

- **v5 closure: one closed frontmatter grammar, no untyped behavioral prose**
  ([ADR-0026](docs/decisions/0026-v5-closed-frontmatter-grammar.md),
  [ADR-0027](docs/decisions/0027-v5-no-untyped-behavioral-prose.md)).
  `.tfer` is now the sole source format, parsed by TypeFerence's own closed
  indentation grammar with syntactic scalar typing: quoted strings,
  arbitrary-precision integers, verbatim decimal lexemes (`0.50` ≠ `0.5`),
  reserved booleans/null, every other bare word an error; no floating-point
  constructor exists. A field is exactly one of typed context, reference, or
  inert metadata — `workingNorms` is deleted from every kind, and normative
  prose reaches a model only as context resources of a declared contextType.
  `description` is inert metadata and emitters keep it out of model-facing
  output. The conformance corpus, helio, and the self-hosted maintainer
  definition are migrated; digests were regenerated via `-update`.
- **Closed v4 source language** ([ADR-0020](docs/decisions/0020-close-the-source-language.md)).
  Native context types replace embedded JSON Schema, context values and typed
  slots are complete compile-time values, `contextFiles` is removed, mode
  requirements remain conditional, sealed abstract requirements fail, and
  normal product entrypoints reject legacy v3 input.
- **Closed serialization and clarified v4 boundaries**
  ([ADR-0023](docs/decisions/0023-tfer-source-format.md),
  [ADR-0024](docs/decisions/0024-clarify-v4-type-and-composition-boundaries.md)).
  `.tfer` fences and bodied kinds are normative; interfaces remain structural,
  context trust refinement is nominal through explicit embedding, visibility is
  orthogonal to interface satisfaction, tools are independent extern
  dependencies, selected modes are recorded at link, and identical sealed
  diamonds converge.
- **Current-language determinism evidence.** Version 4 golden fixtures now pin
  `.tfer` Unicode, BOM, CRLF, and trailing-newline behavior, schema-directed
  number tokens, cross-platform digest path ordering, and signed, unsigned, and
  fail-closed trust publication. A specification evidence matrix tracks the
  normative test surface, and resolver responsibilities are separated by phase.
- **Build/link separation and source identity**
  ([ADR-0022](docs/decisions/0022-build-link-and-source-identity.md)). Build emits
  deterministic unlinked targets and integrity indexes; explicit deployment
  files bind tools, modes, commands, environment references, and endpoints.
  Linked Codex MCP configuration and neutral A2A cards are structurally
  serialized without changing source or unlinked-target identity. Link now
  replaces a non-empty output only when valid root provenance identifies a prior
  TypeFerence link ([ADR-0025](docs/decisions/0025-own-linked-output-before-reset.md)),
  and the stdio `{bundle}` projection is specified explicitly.
- **Go-only implementation** ([ADR-0014](docs/decisions/0014-go-only-implementation.md)).
  The C# reference implementation was retired; the Go implementation is now the
  sole implementation. The specification stays normative in principle. The
  cross-implementation conformance suite becomes a single-implementation
  golden-file determinism suite over the same fixtures — the determinism
  guarantee is unchanged.

### Added

- **Locked source packages and enterprise restore**
  ([ADR-0021](docs/decisions/0021-restore-locked-source-packages.md)):
  `pack`, `restore`, and `update`, canonical `.tferpkg` archives and lockfiles,
  complete offline dependency materialization, scoped filesystem/HTTP/JFrog/
  Azure Artifacts routes, and dependency provenance in target artifacts.
- **`.tfer` source format** and object-model constructs (ADRs 0012–0019, 0023–0024):
  invocation-mode skill variants, user-defined typed context (`contextType`
  refinement, context held by id, `requiresContextTypes`), tools as extern
  declarations (`requiresTools`), capability exposure/visibility, and sealing
  (`sealed`/`required` bindings). All additive and opt-in.
- **Language server** (`typeference-lsp`): authoring and composition
  diagnostics, completion, go-to-definition, and document symbols for `.tfer`
  and `.yaml` sources.

## [0.0.4] - 2026-07-15

### Added

- **CLI usage section** in `README.md` — a self-contained command reference
  (`validate`, `build`, `inspect`, `diff`, `eval`, `equivalence pack`/`score`,
  `version`) for anyone running the binary from a release archive rather than
  a full clone; `README.md` ships in every release archive alongside the
  binary.

## [0.0.3] - 2026-07-15

First tagged release. Everything before this version was unversioned development.

### Added

- **Go implementation** under `go/` — a second, independent implementation of the
  specification producing byte-identical artifacts to the C# reference
  implementation. Ships as a single static binary (`CGO_ENABLED=0`) for
  linux/darwin/windows on amd64 and arm64; only dependency is `gopkg.in/yaml.v3`
  (ADR-0003).
- **Cross-implementation conformance suite** under `conformance/` — 26
  language-neutral fixtures whose expected digests are generated by the Go runner
  and independently verified by the C# runner; both run in CI (ADR-0005).
- **Self-hosted maintainer agent** under `agents/maintainer/` — the repository's
  own maintainer defined in TypeFerence; the root `AGENTS.md` and
  `dist-maintainer/` are compiled artifacts, with a CI gate that fails on drift
  (ADR-0006).
- **Behavioral eval harness** (`typeference eval`, Go implementation) — scenario
  files with expected-behavior rubrics, LLM-judged adherence scoring, and a
  dry-run default that emits exact request payloads with no network calls
  (ADR-0008). Starter corpus under `evals/`.
- **Browser playground** under `web/playground` — the unmodified Go compiler
  built for `js/wasm` against an in-memory filesystem, with a dependency-free
  UI: live recompilation, artifact browser, embedding graph, resolved-bundle
  view, and shareable links. Deployed to GitHub Pages on push to `main`; the
  Helio example reproduces the committed `dist/` digest exactly (ADR-0010).
  Its **Equivalence tab** is a BETH operator console: packs scenario × surface
  cells in the browser with the real `equivalence pack` code, collects
  responses by copy/paste, exports a deterministic run `.tar.gz` for local
  scoring, and visualizes the resulting `scorecard.json` — no credential ever
  touches the page (ADR-0011).
- `typeference version` command in both implementations.
- Architecture decision records under `docs/decisions/`.
- Tag-driven release workflow shipping per-platform archives of the Go CLI with
  a `SHA256SUMS` file (ADR-0007).
- Makefile entry points for build, test, conformance, self-host, and release
  binaries.

### Changed

- **Specification: canonicalization made explicit and portable** (ADR-0004).
  Canonical JSON member order, the escape table, indentation, number-token
  preservation, UTF-8/BOM/CRLF handling, and canonical string ordering (Unicode
  code point order) are now normative. The `typeference-directory-v1` digest
  sorts forward-slash relative paths instead of platform paths, fixing a
  platform-dependent ordering; published digests for the example corpus are
  unchanged.
- Slot names and trust metadata keys must be ASCII identifiers
  (`[A-Za-z0-9][A-Za-z0-9._-]*`); trust identities must be ASCII (punycode for
  internationalized authorities). These are breaking for source trees that used
  non-ASCII keys, accepted without a `schemaVersion` bump under the
  experimental-draft status (ADR-0004).
- The `.yaml` resource-file extension match is byte-exact on all platforms
  (previously case-insensitive on Windows only).
