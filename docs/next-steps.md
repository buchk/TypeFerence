# Next steps

This is the maintained architectural follow-up list. It holds work that is
important but not required to make the current branch semantically coherent.
Completed merge requirements belong in the pull request and changelog instead of
remaining as stale unchecked tasks here.

## Version 7 implementation

ADR-0035 through ADR-0037 and the version 7 specification are ahead of the
compiler. Implement them in the order recorded in
`docs/design-notes/v7-inventory.md`:

1. Remove the cut surfaces in one change.
2. Add skill files and MCP servers with the build-emitted `mcp.json`.
3. Add parameters, field references, and instances.
4. Add Copilot fields and emitted output schemas.
5. Expand `import`.
6. Build `examples/helio-v7` as the committed reference. It replaces
   `examples/helio` and `dist/`.
7. Rework the playground around the Helio v7 example and a form generated from
   a context type.

Each step regenerates the conformance corpus and the maintainer distribution.

## Near term

- [ ] Define stable diagnostic codes for CLI, LSP, and conformance consumers;
  diagnostic text is intentionally not yet contractual.
- [ ] Add a review check that every normative specification edit updates
  `docs/conformance-matrix.md` and, for canonicalization or composition, adds a
  current version 6 golden fixture.
- [ ] Continue separating resolver phases behind narrow internal inputs and
  outputs. The first file-level decomposition keeps composition, context typing,
  interfaces, and dependency validation distinct; a later change can introduce
  explicit phase result types if that improves reviewability without changing
  semantics.
- [ ] Isolate mutable resolver normalization state from loaded source documents.
  Source identity is already computed from canonical source files, but a cloned
  resolver input would make that boundary structural rather than conventional.
- [ ] Replace `samePromotedSkill`'s whole-struct comparison with an explicit
  semantic member identity once that identity is specified. The current
  comparison deliberately ignores only dispatch names and provenance and remains
  conservative for ambiguity detection.
- [ ] Run the Agent Plugins pilot that ADR-0029 requires before acceptance:
  auto-install of repository-enabled plugins in non-interactive (`-p`) runs,
  the `extraKnownMarketplaces` source object for a GitHub-hosted marketplace,
  plugin versus repository agent parity, stdio MCP environment inheritance,
  `--allow-tool` syntax for MCP tools, and plugin version comparison on update.
- [ ] Decide how typed context field values reach hosts that read only
  Markdown. Plugin agent files and `SKILL.md` render a context's title and text
  body; its field values are carried only in `bundle.json`.
- [ ] Move the trust configuration reader onto the closed grammar
  (ADR-0032 decision 1 leaves it on its YAML-based reader).
- [ ] Credentialed tool servers in plugins. Link refuses environment-forwarded
  and bearer-token credentials for a plugin's `mcp.json` (ADR-0029 decision 7);
  specify host-native configuration for credentialed CI use once the pilot
  shows how Copilot supplies credentials to plugin MCP servers.
- [ ] Verify how Copilot CLI authenticates to a private marketplace in a CI
  job, whose default token reads only its own repository (ADR-0034).
- [ ] Automate marketplace pin bumps: when a team publishes a package, open
  a pull request that updates the marketplace package's pin and lockfile.
- [ ] Decide whether `typeference import` should recover shared structure
  (profiles, extensions) from near-duplicate imported skills, or leave that to
  authors (ADR-0033).

## Corpus ownership

- `examples/helio`: integrated product narrative and committed reference output.
- `agents/maintainer`: self-hosting definition and generated-artifact drift gate.
- `conformance/fixtures`: small, isolated normative and canonical-byte cases.

When a feature is added, update only the corpora whose role requires it. A fixture
proves a rule; an example teaches a workflow; a committed distribution proves
deterministic materialization.
