# Next steps

This is the maintained architectural follow-up list. It holds work that is
important but not required to make the current branch semantically coherent.
Completed merge requirements belong in the pull request and changelog instead of
remaining as stale unchecked tasks here.

## Near term

- [ ] Define stable diagnostic codes for CLI, LSP, and conformance consumers;
  diagnostic text is intentionally not yet contractual.
- [ ] Add a review check that every normative specification edit updates
  `docs/conformance-matrix.md` and, for canonicalization or composition, adds a
  current v4 golden fixture.
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
- [ ] Define a third-party target-adapter conformance contract before accepting
  adapters as supported rather than experimental.

## Corpus ownership

- `examples/helio`: integrated product narrative and committed reference output.
- `examples/repo-agent`: compact authoring example that exercises the object
  model; it must not become a second Helio.
- `agents/maintainer`: self-hosting definition and generated-artifact drift gate.
- `conformance/fixtures`: small, isolated normative and canonical-byte cases.

When a feature is added, update only the corpora whose role requires it. A fixture
proves a rule; an example teaches a workflow; a committed distribution proves
deterministic materialization.
