# 0008 — Authoring tools: import, the setup wizard, and the playground

## Status

Accepted (2026-10-07).

## Context

Most organizations already have Copilot agents, skills, and plugins, and
people judge a language by trying it. The tools that get them in must not
become a second semantics.

## Decision

1. **`import` fails closed.** It converts a skill directory, an Agent Plugin,
   a Copilot CLI plugin, or a repository's agents and skills into a new
   package: one plugin listing everything, with metadata, skill files,
   servers, native components, and every other plugin file carried. Anything
   the language cannot represent fails the import and is listed item by item;
   `--lossy` drops those items and lists them. Names are never rewritten.
   Import writes only into an empty directory, never touches the network, and
   validates its output with the compiler. A marketplace repository is not
   imported whole; import lists its plugins. After import, the `.tfer` files
   are the source; nothing round-trips.
2. **One scaffold generator, two front doors.** `internal/scaffold` maps a
   versioned answer set to a source tree. `typeference init` and the browser
   wizard both call it, and `--verify` checks a tree against its digest. The
   wizard owns no semantics: its output compiles with no wizard context.
3. **The playground runs the real compiler.** `typeference-wasm` exposes the
   ordinary build pipeline over an in-memory filesystem in the browser, with
   no backend; Helio reproduces the committed `dist/` digest there. Generated
   assets (`typeference.wasm`, `examples.json`, `wasm_exec.js`) are built, not
   committed, and CI builds the bridge on every push.

## Consequences

- Existing content reaches a marketplace in one command plus review, and
  anything left behind is visible.
- Import does not infer composition; factoring shared profiles and extensions
  out of imported content is deliberate authoring work.

## Alternatives considered

- **Best-effort conversion.** Silent loss, and a dropped tool restriction can
  change what an agent may do.
- **Rewriting invalid names.** It would change slash commands people use.
- **Scaffolding in JavaScript.** A second semantics that would drift.
