# 0007 — Release distribution: static binaries, no installer

**Status:** Accepted (2026-07-08)

## Context

The first-run experience should not require the .NET SDK, a package manager, or an
installer. The Go implementation compiles to a single static binary with no runtime
dependencies, which makes "download one file, run it" the natural distribution. The
question was what a release artifact should be: bare executables, archives, or a
platform installer (MSI on Windows).

## Decision

- **Releases ship per-platform archives of the single static binary** — `.zip` for
  Windows, `.tar.gz` for Linux/macOS — each containing `typeference(.exe)`,
  `LICENSE`, and `README.md`, plus one `SHA256SUMS` file over all archives.
  Supported platforms: linux/darwin/windows × amd64/arm64.
- **No MSI or other installer.** The binary needs no installation: no runtime, no
  registry entries, no shared state. An installer would add packaging and signing
  infrastructure while solving nothing — an unsigned MSI triggers the same Windows
  SmartScreen friction as a bare download. Users place the binary on `PATH`
  themselves, which is the established convention for single-binary CLIs
  (terraform, kubectl, gh).
- **Releases are tag-driven and gated.** Pushing a `v*` tag runs
  `.github/workflows/release.yml`: both test suites, the conformance suite, and the
  self-host drift gate must pass at the tagged commit; the released linux binary is
  smoke-tested against the committed reference output before the GitHub Release is
  created (`gh release create`, `--prerelease` for `v0.*` tags).
- **Version identity.** The tool version lives in the git tag; the Go binary is
  stamped at build time (`-X main.version`), the C# assembly takes
  `Directory.Build.props` `<Version>`, and both CLIs expose a `version` command.
  Tool versions are independent of the typed-resource `schemaVersion` (see
  `docs/release-checklist.md`).
- The version is **never** embedded in compiled agent artifacts — output bytes
  depend only on source, per the determinism rules.

## Alternatives considered

- **MSI / platform installers** — rejected: packaging and code-signing overhead
  with no benefit for a dependency-free single binary; unsigned installers still
  trigger SmartScreen.
- **Bare executables as release assets** — rejected: no place for LICENSE, and
  browsers/AV treat bare `.exe` downloads worse than archives.
- **goreleaser** — rejected for now: the release is a ~30-line shell loop; a
  third-party release toolchain is not warranted at this size (stdlib-first
  policy, ADR-0003). Revisit if package-manager manifests (winget, scoop,
  homebrew) are added.
- **Publishing the C# CLI as a release artifact** — deferred: it would require
  per-platform .NET runtime packaging (self-contained publish) for a CLI that is
  byte-for-byte equivalent to the Go one; the reference implementation remains a
  build-from-source artifact.
