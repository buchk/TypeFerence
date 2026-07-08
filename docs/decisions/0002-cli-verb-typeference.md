# 0002 — The CLI verb is `typeference`

**Status:** Accepted (2026-07-08)

## Context

The project name is Type + Inference ("TypeScript for inference"). The reference
implementation already ships an executable whose assembly name is `typeference`, but the
README invoked it through a PowerShell variable named `$tf`, which read as if the CLI
verb were `tf` — a verb that collides with the common Terraform alias. With a second
implementation arriving, the verb had to be fixed once, deliberately, and used
identically by both implementations, all documentation, and all fixtures.

## Decision

The canonical CLI verb is **`typeference`** for both the C# reference implementation and
the Go implementation. The Go binary is built from `go/cmd/typeference`. Documentation
must never introduce shorthand aliases that could be mistaken for the verb itself.

## Consequences

- No churn: the shipped assembly name, the committed `.codex/config.toml` artifacts
  (`command = "typeference"`), and the MCP server wiring already use this name.
- The README quick start is rewritten to call the binary by its real name.
- Anyone who wants a shorter verb can alias locally; the project does not bless one.

## Alternatives considered

- **`typefer`** — shorter, still evokes Type+inFERence. Rejected: renaming the shipped
  assembly and every committed artifact buys no correctness, and the truncated form
  reads as a typo of "typo-fer" as easily as Type+Inference.
- **`tyf` / other 3-letter forms** — rejected: cryptic, loses the etymology entirely,
  and short verbs are exactly how the `tf`/Terraform confusion started.
- **`tf`** — rejected outright: collides with the ubiquitous Terraform alias.
