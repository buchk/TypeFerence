# 0010 — Browser playground: the Go compiler as WebAssembly

## Status

Accepted.

## Context

Evaluating TypeFerence required installing a toolchain: clone the repository,
build the Go binary (or the .NET solution), then run `validate` / `build`
against an example. That is a high price for the first five minutes of
curiosity, and it keeps the project's two strongest properties abstract:

- **Determinism.** "Byte-identical artifacts" is a claim in the README until a
  person watches the same digest come out of two different environments.
- **Composition.** The embedding/promotion model is easiest to understand by
  editing a profile and watching the compiled `AGENTS.md` change.

The project already pays for two independent implementations kept
byte-identical by a conformance suite (ADR-0005). That investment makes a
browser port nearly free: Go compiles to `js/wasm` out of the box, and the Go
implementation's only dependency is `gopkg.in/yaml.v3` (ADR-0003), which is
pure Go.

## Decision

Ship a static, zero-install playground at `web/playground`, deployed to GitHub
Pages on every push to `main` (`.github/workflows/pages.yml`).

1. **The compiler is not forked, wrapped, or reimplemented.** A new
   `go/cmd/typeference-wasm` entry point builds the existing `internal/`
   packages for `GOOS=js GOARCH=wasm` and exposes one function,
   `TypeFerence.compile`, that runs the same `compile.Validate` →
   `compile.Build` → `compile.HashDirectory` pipeline as the CLI.
2. **File I/O is satisfied, not removed.** Go's `js/wasm` syscall layer
   delegates to a global Node-style `fs` object. `web/playground/memfs.js`
   implements that API over an in-memory tree, so the compiler's ordinary
   `os` calls work unchanged inside the browser tab.
3. **The UI has zero dependencies.** Plain HTML/CSS/JS: an editor with
   lightweight syntax coloring, an artifact browser, the embedding graph
   (including computed structural-interface satisfaction), the resolved-bundle
   view, and a share link (gzip + base64url in the URL fragment). Nothing the
   user types leaves the tab; there is no backend.
4. **Digests reproduce the repository's own builds.** The in-memory source
   directory is named after the loaded example and the ARD publisher domain
   matches the repository's build commands, so the Helio example produces the
   exact digest of the committed `dist/`, and the maintainer example
   reproduces the repository-root `AGENTS.md` byte for byte.
5. **Generated assets are not committed.** `typeference.wasm`,
   `examples.json`, and `wasm_exec.js` are produced by `make playground`;
   `wasm_exec.js` is copied from the building toolchain's `GOROOT` so it can
   never skew from the wasm binary's Go version. CI builds the wasm bridge and
   packer on every push so the playground cannot silently rot.

## Consequences

- Anyone can evaluate TypeFerence — including the determinism guarantee — from
  a link, with nothing installed and no data leaving their machine.
- The playground is a demonstration surface, not a supported runtime. The CLI
  remains the product; the playground intentionally exposes no command surface
  beyond compile-on-edit.
- The wasm binary is ~5 MB (~1.8 MB compressed), an acceptable one-time load
  for a code playground.
- `memfs.js` implements the private contract between Go's syscall layer and
  its JavaScript host. A Go major release could change that contract; because
  `make playground` builds binary and shim support files from one toolchain,
  such a break surfaces at build/CI time, not silently in production.

## Alternatives considered

- **A TypeScript reimplementation of the compiler for the browser.** A third
  implementation to keep conformant forever, and it would demonstrate nothing
  about the shipped compilers. Rejected outright; the whole value of the
  playground is that it runs the real one.
- **A hosted compile API.** Requires a server, introduces cost, latency, and a
  privacy story ("your agent definitions are uploaded to..."), and undermines
  the static-artifact ethos of the project. The wasm build is strictly better.
- **Compiling the C# reference implementation with .NET's wasm support.**
  Works, but produces a far larger payload and a heavier runtime; the Go
  implementation exists precisely to be the small, static, portable one
  (ADR-0007).
- **Recorded demo (GIF/asciinema) in the README.** Cheap, but not interactive
  and proves nothing — a recording of a digest is not a digest you produced.
