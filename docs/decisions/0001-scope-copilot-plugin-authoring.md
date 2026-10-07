# 0001 — Scope: an authoring and reuse layer for Copilot plugins

## Status

Accepted (2026-10-07).

## Context

An organization that standardizes on GitHub Copilot wants to write its
skills, agents, and MCP servers once, specialize them per team, and ship them
through one marketplace without names colliding or copies drifting. People
use that expertise two ways: by installing plugins interactively, and in
pipelines that run Copilot CLI with a skill and validate its result against a
schema.

Copilot already installs, enables, authenticates, and runs Agent Plugins 1.0
packages, and reads marketplaces from `.github/plugin/marketplace.json`. What
the format cannot express is reuse across plugins, checks across a whole
marketplace, and one reviewable source for every emitted byte.

TypeFerence started as a general typed model of the agent ecosystem: neutral
bundles for any host, A2A and ARD publication, deployment linking, signed
packages, structural interfaces, a behavioural equivalence harness, and a ban
on untyped prose. Nobody asked for that platform, and several of its
guarantees defended against things that cannot be prevented (a sealed skill
can be replaced by writing a new one; a model accepts any text a user types).

## Decision

1. TypeFerence compiles source packages into Agent Plugins 1.0 directories and
   a Copilot marketplace index. GitHub Copilot is the only output adapter, and
   `agent-plugin` is the only build target. Copilot and GitHub install, host,
   enable, authenticate, and run what it emits.
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
- A second host later means a new adapter; the Agent Plugins core stays the
  default and Copilot-only fields are explicit opt-ins (ADR-0007).
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
