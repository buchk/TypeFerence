# 0008 — Authoring tools: import, the setup wizard, the playground, and the compiler plugin

## Status

Accepted (2026-10-07). Amended 2026-10-19: the compiler ships as a plugin.

## Context

Most organizations already have Copilot agents, skills, and plugins, and
people judge a language by trying it. Increasingly they author through an
agent rather than an editor. The tools that get them in must not become a
second semantics.

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
4. **The compiler ships as a plugin.** `typeference mcp` serves the CLI's
   commands as MCP tools over stdio. Each tool call runs the same binary with
   the same arguments a person would type and returns its output and exit
   code, so the server owns no semantics. `agents/typeference` is an ordinary
   package whose plugin holds an authoring skill and that server and nothing
   Copilot-specific, so it is a portable plugin. Its build is committed in
   `dist-plugin/`, and the repository root's `.github/plugin/marketplace.json`
   is that build's index with each plugin source prefixed by the committed
   path, which makes this repository a marketplace. `make selfhost`
   regenerates both and CI fails on drift, as it does for `AGENTS.md`. The
   server is a bare `typeference` command: installing the plugin does not
   install the compiler, and the skill says how to.

## Consequences

- Existing content reaches a marketplace in one command plus review, and
  anything left behind is visible.
- Import does not infer composition; factoring shared profiles and extensions
  out of imported content is deliberate authoring work.
- An agent with the plugin installed can stand up a package, its
  dependencies, and its marketplace, and checks its work with the same
  validate, build, and diff a person runs.
- The root marketplace index is derived outside build, like `AGENTS.md`: the
  path where this repository commits its build output is a fact about this
  repository, not about the package.

## Alternatives considered

- **Best-effort conversion.** Silent loss, and a dropped tool restriction can
  change what an agent may do.
- **Rewriting invalid names.** It would change slash commands people use.
- **Scaffolding in JavaScript.** A second semantics that would drift.
- **MCP tools that call the compiler's packages directly.** Commands print to
  standard output, which is the MCP channel, and a second entry point to
  each command would drift from the CLI.
- **Compiler binaries inside the plugin.** Every install would carry a binary
  per platform, and a committed artifact would hold executables.
- **A WebAssembly server.** One portable file, but it needs a runtime the
  client may not have, and the filesystem access build needs.
