# Next steps

This is the maintained architectural follow-up list. It holds work that is
important but not required to make the current branch semantically coherent.
Completed merge requirements belong in the pull request and changelog instead of
remaining as stale unchecked tasks here.

## Version 7 verification

CI verified the branch: tests, vet, gofmt, the conformance corpus on three
platforms, the Helio reference build, and self-host drift. Still to do by
hand: `make playground`, then exercise the Helio example and the Instantiate
tab in a browser.

## Version 7 follow-ups

- [ ] Copilot behaviours to confirm for native components (ADR-0040):
  plugin rules active by default and loaded in `-p` runs, and plugin hooks
  on Copilot cloud agent.
- [ ] Hooks that run scripts shipped inside the plugin, once the variable that
  names a hook's plugin directory is confirmed.
- [ ] Decide how `list<string>` fields render if a real case needs them in
  text.
- [ ] Decide whether one agent may bind two values to one parameter name.
- [ ] A hosted form UI that writes a data document and opens a pull request
  against a team's package (ADR-0036).
- [ ] Copilot behaviours to confirm in a pilot: repository-enabled plugin
  auto-install in non-interactive runs, and OAuth sign-in for plugin MCP
  servers with the organization's identity provider.

## Near term

- [ ] Define stable diagnostic codes for CLI, LSP, and conformance consumers;
  diagnostic text is intentionally not yet contractual.
- [ ] Add a review check that every normative specification edit updates
  `docs/conformance-matrix.md` and, for canonicalization or composition, adds a
  version 7 golden fixture.
- [ ] Isolate mutable resolver normalization state from loaded source documents.
  Source identity is already computed from canonical source files, but a cloned
  resolver input would make that boundary structural rather than conventional.
- [ ] Verify how Copilot CLI authenticates to a private marketplace in a CI
  job, whose default token reads only its own repository (ADR-0034).
- [ ] Automate marketplace pin bumps: when a team publishes a package, open
  a pull request that updates the marketplace package's pin and lockfile.
- [ ] Decide whether `typeference import` should recover shared structure
  (profiles, extensions) from near-duplicate imported skills, or leave that to
  authors (ADR-0033).

## Corpus ownership

- `examples/helio`: the integrated product narrative; `dist/` is its committed reference output.
- `agents/maintainer`: self-hosting definition and generated-artifact drift gate.
- `conformance/fixtures`: small, isolated normative and canonical-byte cases.

When a feature is added, update only the corpora whose role requires it. A fixture
proves a rule; an example teaches a workflow; a committed distribution proves
deterministic materialization.
