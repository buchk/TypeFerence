# 0003 — Go implementation layout and dependency policy

**Status:** Accepted (2026-07-08)

## Context

The spec exists so that independent implementations can produce byte-identical
artifacts. The second implementation is written in Go, chosen for a single static
binary (`CGO_ENABLED=0`, no runtime dependencies) that fixes the first-run experience:
the reference implementation requires the .NET 10 SDK.

## Decision

- The Go implementation lives in `go/` as its own module, `github.com/buchk/TypeFerence/go`,
  with the CLI at `go/cmd/typeference` and internals under `go/internal/`:
  `resource` (loading/validation), `resolve` (type system), `compile` (targets, digest,
  ARD), `trust` (trust configuration and signatures), and `jsonx` (canonical JSON).
- **Standard library first.** The only third-party dependency is `gopkg.in/yaml.v3`,
  because the source format is YAML and writing a YAML parser is out of scope. Any
  future dependency needs an ADR.
- **`jsonx` instead of `encoding/json`.** The canonical artifact serialization
  (see ADR-0004) requires an exact escape table, member ordering, raw number-token
  preservation, and duplicate-key round-tripping. `encoding/json` guarantees none of
  these; a purpose-built writer/parser (~500 lines, fully tested) does.
- YAML documents are decoded by walking `yaml.Node` trees with an explicit field
  table rather than struct tags. This reproduces the reference implementation's strict
  deserialization behavior (unknown fields rejected, scalars coerced to strings,
  single-document files) without depending on either library's convention defaults.

## Consequences

- The Go module is versioned with the repository; `go install
  github.com/buchk/TypeFerence/go/cmd/typeference@latest` works once the module is on a
  published main branch.
- Byte-compatibility hazards are concentrated in `jsonx` and `compile`, which is where
  the conformance suite (ADR-0005) aims its unicode and canonicalization fixtures.

## Alternatives considered

- **Single multi-language repo module at root** — rejected: `go/` keeps Go tooling
  (gofmt, vet, test) scoped and mirrors how the C# implementation lives in `src/`.
- **`encoding/json` with post-processing** — rejected: patching stdlib output
  (re-escaping, re-ordering) is strictly more fragile than emitting the right bytes
  in the first place.
- **A YAML subset parser written in-repo** — rejected for scope; revisit only if
  yaml.v3 behavior diverges from YamlDotNet on spec-conforming input (any such
  divergence is a spec bug to fix with a fixture; see ADR-0004).
