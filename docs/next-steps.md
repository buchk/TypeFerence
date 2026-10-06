# Next steps

This is the maintained architectural follow-up list. It holds work that is
important but not required to make the current branch semantically coherent.
Completed merge requirements belong in the pull request and changelog instead of
remaining as stale unchecked tasks here.

## Version 7 verification

The version 7 implementation landed on `feat/v7-plugin-authoring` without a
local Go toolchain. Before merging:

1. `go vet ./...` and `go test ./...` from `go/`; fix compile errors and
   failures.
2. `go test ./conformance -update`, then review every success fixture's
   output and confirm every error fixture fails for the stated reason (`-v`
   logs each diagnostic).
3. `make reference` to write `dist/`, and review it against
   `examples/helio/README.md`'s predicted output.
4. `make selfhost` to write `dist-maintainer/` and the root `AGENTS.md`, and
   compare the result with the hand-rendered `AGENTS.md` on the branch.
5. `make playground` and exercise the Helio example and the Instantiate tab.

## Version 7 follow-ups

- [ ] Hooks, commands, rules, LSP configuration, and agent-scoped
  `mcp-servers` in the Copilot extension namespace (ADR-0037 decision 6).
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
