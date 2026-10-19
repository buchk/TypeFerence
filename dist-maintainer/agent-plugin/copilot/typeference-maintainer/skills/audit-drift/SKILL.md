---
name: audit-drift
description: "Confirms the committed AGENTS.md and maintainer plugin are exact build artifacts of this definition."
---

Run `make selfhost-check` from the repository root. It rebuilds this
definition, diffs the result against dist-maintainer, and byte-compares the
repository-root AGENTS.md with the agent file it is generated from. Report
clean=true only when both checks pass, as JSON matching
references/output.schema.json. Any drift between the definition and its
committed artifacts is a broken build: regenerate with `make selfhost` and
commit definition and artifacts together, or revert the stray edit to the
generated files.
