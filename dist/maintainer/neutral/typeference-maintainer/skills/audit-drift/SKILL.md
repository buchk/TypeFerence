---
name: audit-drift
description: "Confirms the committed AGENTS.md and maintainer bundle are exact build artifacts of this definition."
---

Run `typeference diff agents/maintainer --against dist/maintainer --target neutral
--emit-ard --publisher-domain typeference.example` from the repository root, then
byte-compare the repository-root AGENTS.md against
dist/maintainer/neutral/typeference-maintainer/AGENTS.md. Report clean=true only
when the diff exits 0 and the byte comparison matches. Any drift between the
definition and its committed artifacts is a broken build: regenerate with
`make selfhost` and commit definition and artifacts together, or revert the
stray edit to the generated files.

## Context loaded on invocation

- `context/determinism.md`
