# Release checklist

Releases ship the Go CLI as single static binaries per platform (see ADR-0002).
The release version is the git tag; the Go binary receives it at build time via
`-ldflags -X main.version`. `CHANGELOG.md` must contain the matching dated
release entry before that tag is created.

## Before tagging

1. On `main`, CI fully green: the Go test suite, the version 8 conformance
   corpus, the Helio reference test, and the self-host drift gate.
2. `CHANGELOG.md`: move the `Unreleased` heading to the release date; confirm every
   spec-affecting entry names its ADR.
3. Quick start in `README.md` executed literally from a clean clone.
4. No uncommitted generated artifacts: `make selfhost-check` passes, and
   `go test ./internal/compile -run TestHelioReference` reproduces `dist/`.

## Tagging

```
git tag vX.Y.Z
git push origin vX.Y.Z
```

The `release` workflow then: re-verifies tests, conformance, and drift at the
tagged commit; cross-compiles `typeference` for linux/darwin/windows on
amd64/arm64; smoke-tests that the released linux binary reproduces the committed
maintainer plugin; and publishes a GitHub Release with per-platform archives and
`SHA256SUMS`. Tags starting `v0.` are marked as pre-releases.

## After the release

1. Verify the release page lists 6 archives + `SHA256SUMS` and the generated notes.
2. Download one archive on a machine you did not build on; check
   `sha256sum -c SHA256SUMS` (for that file) and `typeference version`.
3. Start the next `Unreleased` section in `CHANGELOG.md`.

## Versioning notes

- Tool releases (this checklist) version the CLIs and libraries. They do **not**
  version the source format: manifests stay `schemaVersion: 8` until an
  incompatible format change, which requires a specification change and an ADR
  first.
- Pre-1.0, breaking tool changes are allowed in any release but must be listed
  under **Changed**/**Removed** in the changelog with their ADR.
