# Contribution workflow

- Work on feature branches; never rewrite published history; never tag or publish a
  release outside the checklist in `docs/release-checklist.md`.
- Before any commit: `dotnet test TypeFerence.slnx`, `go test ./...` (from `go/`),
  and the conformance suite (`make conformance`) all pass. A commit that breaks any
  of them does not land.
- Design decisions with real tradeoffs — spec semantics, canonical bytes, trust
  model, dependencies — are recorded in `docs/decisions/` as numbered ADRs in the
  same change.
- Generated artifacts (root `AGENTS.md`, `dist/`) are only ever changed by
  regenerating them from source (`make selfhost`, `typeference build`); hand edits
  to generated files are drift and CI rejects them.
- Documentation is accurate against the code at the commit that includes it. The
  project describes itself as an experimental reference implementation; no invented
  adoption, users, benchmarks, or endorsements, ever.
