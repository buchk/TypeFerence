# 0001 — Scope: an authoring and reuse layer for Agent Plugins

## Status

Accepted (2026-10-07). Amended 2026-10-19: the portable core is the default
output and Copilot components are named under a `copilot` keyword.

## Context

An organization that standardizes on GitHub Copilot wants to write its
skills, agents, and MCP servers once, specialize them per team, and ship them
through one marketplace without names colliding or copies drifting. People
use that expertise two ways: by installing plugins interactively, and in
pipelines that run Copilot CLI with a skill and validate its result against a
schema.

Copilot already installs, enables, authenticates, and runs Agent Plugins 1.0
packages, and reads marketplaces from `.github/plugin/marketplace.json`.
Agent Plugins 1.0 is a vendor-neutral floor: a manifest, Agent Skills, and MCP
servers, with client-specific components under reverse-domain directories
such as `com.github.copilot/`. Skills and MCP servers written for one client
are worth reusing in others; custom agents, rules, hooks, commands, and
language servers are read by Copilot alone. What the format cannot express is
reuse across plugins, checks across a whole marketplace, and one reviewable
source for every emitted byte.

TypeFerence started as a general typed model of the agent ecosystem: neutral
bundles for any host, A2A and ARD publication, deployment linking, signed
packages, structural interfaces, a behavioural equivalence harness, and a ban
on untyped prose. Nobody asked for that platform, and several of its
guarantees defended against things that cannot be prevented (a sealed skill
can be replaced by writing a new one; a model accepts any text a user types).

## Decision

1. TypeFerence compiles source packages into Agent Plugins 1.0 directories and
   a marketplace index, and `agent-plugin` is the only build target. The
   portable core (`plugin.json`, skills, `mcp.json`) is what every build
   emits. A client-specific component is emitted only where source names it
   under that client's keyword; `copilot` is the only one, and it ships
   custom agents, rules, commands, hooks, and LSP servers into
   `com.github.copilot/`. Nothing client-specific is inferred, dropped, or
   translated between clients, and no build option changes what an artifact
   contains. The marketplace index is the format Copilot documents, because
   Agent Plugins 1.0 leaves distribution to clients. Clients install, host,
   enable, authenticate, and run what TypeFerence emits.
2. A feature belongs only if it serves at least one of:
   - reuse that Markdown cannot express;
   - checks the plugin format cannot express;
   - one marketplace build with one version of each package;
   - a reviewable diff of every emitted byte.
3. Guarantees catch accidents, not intent: colliding names, a skill shipped
   without its server, an unfilled template field, a missing output contract,
   non-deterministic output. One provenance rule replaces typed prose: every
   emitted byte comes from a file in the package closure, and provenance
   records which one.
4. Removed surfaces are deleted, not deprecated. Only the current source
   version is accepted, and no migrator ships, because nothing outside this
   repository used earlier versions.

## Consequences

- The language, the compiler, and the fixture corpus are as small as the job.
- A plugin made only of skills, servers, and data is a portable package any
  Agent Plugins client reads, and its source says so: no `copilot` block.
- A second client later means a second keyword beside `copilot`, with its own
  components and checks; nothing written for Copilot silently becomes
  something else.
- The specification describes the current language only. Earlier designs
  survive in git history.

## Alternatives considered

- **Build on Microsoft's APM.** APM installs and governs agent configuration
  per repository. An organization that relies on GitHub's marketplace would
  use only its producer side, which does not emit Copilot's marketplace
  format, cannot combine agents with MCP configuration in one plugin, and has
  no extension or templating. TypeFerence's output is plain plugins, so APM
  can still install them.
- **Keep the general platform frozen beside the Copilot target.** Every
  fixture, test, and document would keep paying for consumers that do not
  exist.
- **A build option that selects a client adapter.** One source would compile
  to different bytes per flag, provenance would need to record the flag, and
  a portable build would have to drop Copilot hooks and rules, so a plugin
  whose policy lives in a hook could ship without it. Naming client
  components in source keeps output a function of source alone, and a
  component outside its keyword is a build error rather than a silent drop.
- **Copilot components emitted by default.** A plugin would be non-portable
  without its author asking, and nothing in the source would show it.
