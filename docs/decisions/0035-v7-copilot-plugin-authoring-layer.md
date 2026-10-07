# 0035 — Version 7: an authoring and reuse layer for Copilot plugins

## Status

Proposed (2026-10-06). On acceptance: amends the specification to
`schemaVersion: 7`. Folds in the still-proposed ADR-0029 through ADR-0034 as
amended here. Companions: ADR-0036 (documents, data, and templates) and ADR-0037
(build-emitted host configuration). Inventory and reasoning:
[`docs/design-notes/v7-inventory.md`](../design-notes/v7-inventory.md).

## Context

TypeFerence began as a typed coherence layer for organizational agents in
general. It covered neutral bundles for any host, A2A and ARD publication,
deployment linking, signed source packages, structural interfaces, a
behavioural equivalence harness, and a fully typed source language with no
untyped prose. Version 6 then made GitHub Agent Plugins the primary output.

An organization that standardizes on GitHub Copilot wants one thing from the
language: write its skills, agents, and MCP servers once, specialize them per
team, and ship them through one marketplace. They should not collide or
drift, and Copilot plugins should be installable everywhere. Such an
organization typically has two paths into the same packaged expertise:

- people installing plugins interactively;
- pipelines running Copilot CLI with a skill against trusted evidence, where a
  runner validates the skill's result against a versioned schema.

Nobody has asked for the general platform. Several of its guarantees also
defend against something that can't be prevented. A sealed skill can be
replaced by writing a new one. A ban on untyped prose blocks nothing, because
a model accepts any text a user types. Meanwhile, things such organizations
need immediately have no home in version 6:

- reference files beside a skill;
- per-team instances of a shared template;
- MCP configuration emitted by build;
- output contracts the runner can see.

Microsoft's APM (Agent Package Manager) was evaluated as a foundation instead.
APM installs and governs agent configuration in every repository. An
organization that relies on GitHub's marketplace and Copilot's own installer
would use only APM's producer side. That side does not emit
Copilot's marketplace format, cannot combine agents with MCP configuration in
one plugin, and has no extension or templating. Its contribution process also
gates new features on maintainer review capacity.

## Decision

1. **Scope.** TypeFerence is an authoring and reuse layer for Agent Plugins,
   with GitHub Copilot as its only output adapter. It compiles skills, agents,
   documents, typed data, and MCP servers into Agent Plugins 1.0 directories
   and a Copilot marketplace index. Copilot and GitHub install, enable,
   authenticate, and run what it produces. Every feature must serve at least
   one of:
   - reuse that Markdown cannot express;
   - checks the plugin format cannot express;
   - one marketplace build with one version of each package;
   - a reviewable diff of every emitted byte.
2. **One build target.** `agent-plugin` is the only target. Build emits
   complete artifacts, including `mcp.json` (ADR-0037). There is no link
   phase. The pipeline is restore, build, then publish outside TypeFerence.
3. **Removed.** The following are deleted from the specification and the
   implementation rather than deprecated. No migrator ships, because nothing
   outside this repository uses version 6.
   - the `neutral` target;
   - ARD catalogs and callable publication;
   - A2A, the `a2a` mode, and capability `visibility`;
   - deployment files, `link`, and `publish`;
   - trust metadata and signature import;
   - interfaces and slots;
   - `allowedContextTypes`;
   - the `tool` kind (replaced by servers, ADR-0037);
   - skill and binding `sealed`;
   - context-type refinement (`embeds`, nominal trust boundaries, field
     redeclaration rules), and the `map<T>` and `decimal` types;
   - context-type `body`;
   - the behavioural evaluation and equivalence commands;
   - the archival v5 and v3 languages and their corpora.
4. **Guarantees narrowed to ones that catch accidents.** Version 7 keeps the
   checks that stop mistakes from reaching people:
   - names that collide anywhere in a marketplace;
   - a skill shipped without the server it needs;
   - a template field that is missing or unfilled;
   - an output contract that is not emitted;
   - non-deterministic output.

   It drops guards against authors deliberately changing behaviour. ADR-0027's
   field-classification rule and its ban on untyped prose are replaced by one
   provenance rule: every emitted byte comes from a file in the package
   closure, and provenance records which one. `description` stays routing
   metadata, because that is Copilot's semantics.
5. **Kept as specified in version 6:**
   - the closed `.tfer` grammar and schema-directed scalars;
   - path-derived identity;
   - multi-plugin packages and plugin documents;
   - profiles and embedding with shallowest-wins promotion;
   - additive skill extension;
   - capabilities, bindings, and `required`;
   - the `manual` and `pipeline` modes;
   - source packages, restore, and lockfiles;
   - one organization marketplace built from packages, and candidate
     validation;
   - the compatibility report;
   - canonicalization, digests, and `diff`;
   - `import`;
   - `init`;
   - the language server;
   - the browser playground, reworked as the showcase.
6. **A `git` package route.** Alongside the existing filesystem, HTTP, JFrog,
   and Azure Artifacts routes, a route may resolve packages from a Git
   repository by tag. The lockfile is unchanged: it records the package digest,
   which restore verifies whatever the route.

## Consequences

- The language, the implementation, and the conformance corpus all shrink.
  The inventory estimates about a third of the Go code and about
  three-quarters of the fixtures go. The resolver, packages, plugin emitter,
  and marketplace build remain.
- Every guarantee authors rely on daily is still checked at build time. The guarantees that only constrained intentional authorship are gone.
- Supporting a second host later means adding an adapter. Copilot-only output
  is already explicit opt-in (ADR-0037), and the portable Agent Plugins core
  is the default.
- The whitepaper's broader thesis (A2A, ARD, behavioural equivalence) is
  withdrawn from the product description. It remains in the record through the
  superseded ADRs.

## Alternatives considered

- **Extend APM upstream and retire TypeFerence.** This would be attractive if
  the organization ran APM in every repository. It doesn't, and the producer
  gaps plus the upstream review queue would leave the organization without a
  working tool for months. TypeFerence's output is plain plugins, so APM can
  still install them later.
- **One plugin per package root.** Simpler to explain, but real marketplace
  repositories hold several plugins and a catalog together. Version 6's model
  already fits them.
- **Keep the removed surfaces frozen.** Every fixture, test, and document
  would keep carrying digests and rules for consumers that do not exist.
- **Keep `link` for environment-specific MCP configuration.** Plugin MCP
  configuration cannot carry secrets or rely on environment variables, so the
  common case has nothing environment-specific in it. Revisit if a team
  genuinely needs per-environment server addresses.
