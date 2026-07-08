# Release checklist

Releases ship the Go CLI as single static binaries per platform (see ADR-0007).
The version lives in three places that must agree: `CHANGELOG.md`,
`Directory.Build.props` (`<Version>`), and the git tag (the Go binary takes its
version from the tag at build time via `-ldflags -X main.version`).

## Before tagging

1. On `main`, CI fully green: both test suites, the conformance suite
   (25/25 fixtures on both implementations), and the self-host drift gate.
2. `CHANGELOG.md`: move the `Unreleased` heading to the release date; confirm every
   spec-affecting entry names its ADR.
3. `Directory.Build.props` `<Version>` matches the version being tagged.
4. Quick start in `README.md` executed literally from a clean clone.
5. No uncommitted generated artifacts: `make selfhost-check` passes; `typeference
   diff examples/helio --against dist --emit-ard --publisher-domain helio.example`
   exits 0.

## Tagging

```
git tag vX.Y.Z
git push origin vX.Y.Z
```

The `release` workflow then: re-verifies tests, conformance, and drift at the
tagged commit; cross-compiles `typeference` for linux/darwin/windows on
amd64/arm64; smoke-tests that the released linux binary reproduces the committed
reference output; and publishes a GitHub Release with per-platform archives and
`SHA256SUMS`. Tags starting `v0.` are marked as pre-releases.

## After the release

1. Verify the release page lists 12 archives + `SHA256SUMS` and the generated notes.
2. Download one archive on a machine you did not build on; check
   `sha256sum -c SHA256SUMS` (for that file) and `typeference version`.
3. Start the next `Unreleased` section in `CHANGELOG.md` and bump
   `Directory.Build.props` if the next version is known.

## Versioning notes

- Tool releases (this checklist) version the CLIs and libraries. They do **not**
  version the source format: typed resources stay `schemaVersion: 3` and trust
  configurations `schemaVersion: 1` until an incompatible format change, which
  requires a specification change and an ADR first.
- Pre-1.0, breaking tool changes are allowed in any release but must be listed
  under **Changed**/**Removed** in the changelog with their ADR.
