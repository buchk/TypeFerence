# 0001 — Scope: an authoring and reuse layer for agent plugins

## Status

Accepted (2026-10-07). Amended 2026-10-19: build adapters for Copilot,
Claude Code, and Codex; Copilot components are named under a `copilot`
keyword.

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

1. TypeFerence compiles source packages into plugin packages and marketplace
   indexes, and `agent-plugin` is the only build target. No layout is read by
   every client as written, so build has one adapter per supported client,
   GitHub Copilot, Claude Code, and Codex, and every build runs all three
   from the same resolved plugins. Each adapter writes its client's layout and
   its client's marketplace index, so the target is a marketplace repository
   each client recognizes. Skills, servers, documents, and data are written
   once for every client. A component only one client reads is written under
   that client's keyword; `copilot` is the only one, and it ships custom
   agents, rules, commands, hooks, and LSP servers into `com.github.copilot/`.
   An adapter translates nothing but spelling (a variable name, a path form),
   and a plugin it cannot build as written is not built for that client, with
   the reason recorded, rather than built without part of its composition. No
   build option changes what the target contains. Clients install, host,
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
- A plugin of skills, servers, and data installs in Copilot, Claude Code, and
  Codex from one source, with no client file written by hand.
- A plugin whose policy depends on a Copilot hook or rule is never offered to
  a client that would not run it, and `build.json` says so.
- Custom agents, hooks, and the rest stay Copilot components for now: a
  Copilot agent is a persona, a Claude Code subagent is a delegated worker,
  and their hooks differ in events and blocking. A client's own components
  arrive as that client's keyword, with its own checks, not as translations
  of Copilot's.
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
  a build for another client would have to drop Copilot hooks and rules, so a
  plugin whose policy lives in a hook could ship without it.
- **One Agent Plugins 1.0 package for every client.** Claude Code reads
  neither its manifest nor its `mcp.json`, and each client finds its
  marketplace at a different path, so no single layout installs everywhere.
- **Hand-written indexes for other clients, carried as marketplace files.**
  They duplicate what build already knows and drift, which is the problem
  TypeFerence exists to remove.
- **Translating Copilot agents and hooks for other clients.** The same words
  mean different things there; a translation would change behavior without
  the author seeing it.
