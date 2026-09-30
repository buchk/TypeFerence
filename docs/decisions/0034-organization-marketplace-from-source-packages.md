# 0034 — One organization marketplace, built from source packages

## Status

Proposed (2026-09-30). On acceptance: amends the version 6 specification
("Organization marketplaces"); changes the provenance recorded in plugin
artifacts and the agent-plugin build index. Builds on ADR-0021 (locked source
packages), ADR-0029, ADR-0030, and ADR-0031.

## Context

ADR-0029 through ADR-0031 made every "one name, one thing" guarantee a property
of one build: each emitted skill, agent, and plugin name maps to one resource,
a shared skill is emitted identically wherever it ships, and the compatibility
report names plugins that compete when installed together. Each build also
writes its own marketplace index.

The organization this direction serves authors agents and skills in many
repositories but publishes them to one private marketplace. Copilot pools what
people install from that marketplace: installed skills and agents share one
namespace, and duplicates resolve first-found-wins. Between independent builds
nothing holds the guarantees:

- two repositories can ship different plugins, agents, or skills under one
  name, and which one a person gets depends on load order;
- two teams can ship a shared core skill built from different versions of the
  core package, so one name carries two bodies;
- two teams can extend one core skill, and nothing reports that installing
  both packs makes them compete;
- one marketplace index has to list every plugin, and something has to own it.

The marketplace is also meant to hold only compiler output: a plugin reaches it
by going through TypeFerence, never by being copied in.

## Decision

1. **The marketplace is one build.** It is the build of a *marketplace
   package* whose manifest lists plugins of its direct dependencies as
   `<package>:<path>`. A package may list only plugins that the dependency's
   own manifest lists; a dependency's other plugins never ship. Teams publish
   their packages to the feed with the existing `pack` and restore tooling,
   and the marketplace package pins each at an exact version in its lockfile.
2. **Every build rule spans packages.** Plugin, custom agent, and emitted skill
   names are unique across the whole marketplace; a collision between two
   teams fails the build and names both resources. The compatibility report
   covers every plugin. A shared skill is one skill, emitted identically in
   every plugin that ships it for a given mode, so installing several plugins
   that carry it is harmless.
3. **One version of each package.** The locked graph holds exactly one version
   of every package, as restore has always required (ADR-0021). Two teams that
   depend on different versions of a shared package cannot both ship until one
   of them moves, and the conflict is reported when the marketplace pin is
   bumped, not after people install.
4. **Artifacts belong to their owning package.** A plugin's version is its
   owning package's version, and its link requirements record the owning
   package's source digest and the locked packages in that package's
   dependency closure. A plugin artifact is therefore a function of its owning
   package and that package's locked closure alone: bumping one team's pin
   rewrites only that team's plugin directories, the marketplace index, the
   build index, and the compatibility report. The agent-plugin build index
   records each artifact's owning source digest, and link verifies each
   artifact against its own entry. Neutral bundles keep the build's
   provenance: they are not the published unit, and their format is shared
   with the archival corpora.
5. **Preflight before publishing.** `typeference validate <marketplace>
   --candidate <package-dir>` validates the marketplace with an unpublished
   package substituted for its pinned version, or added. The candidate's
   dependencies must match the locked graph, and every package that depends
   on the candidate's name must already declare the candidate's version. It
   writes nothing, and a candidate is never input to build or link.
6. **Only compiler output reaches the marketplace.** The published marketplace
   repository is the marketplace package's `agent-plugin` target, linked when
   its plugins import tools. Publication replaces the repository's contents,
   so anything else there is drift. There is deliberately no command that merges
   externally built plugins into a marketplace.

## Consequences

- An organization gets the single-build guarantees across every team, and a
  collision, divergent shared skill, or version skew fails in a pull request
  against the marketplace package rather than on someone's machine.
- Every team release becomes a reviewed change to the marketplace package's
  pins. Automation can open that change; the platform team owns the manifest.
- A marketplace diff for one team's release is confined to that team's plugin
  directories plus index files, which keeps it reviewable.
- Shared core packages move in lockstep across the marketplace. That is the
  cost of one body per skill name; the preflight shows exactly which
  dependents must move with a core bump.
- The marketplace build compiles every pinned package with one compiler
  version. A package that fails under a new compiler blocks the marketplace
  until it is fixed or its pin is held back.
- This change alters bytes of every version 6 plugin build index (the new
  per-artifact source digest), so the version 6 corpus's agent-plugin digests
  and the committed reference outputs are regenerated with it. Plugin
  directories owned by the building package are unchanged.

## Alternatives considered

- **Collect built plugins.** Each repository builds and copies its plugin
  directories into the marketplace, and a merge tool writes the index and
  compares bytes. Rejected: checks run on compiled output after the fact,
  version skew surfaces only as "these copies differ", nothing can rebuild the
  marketplace from source, and a merge path is exactly the side door the
  marketplace must not have.
- **Qualify every emitted name by package** (`core-repository-status`).
  Rejected: slash commands and agent names get worse for everyone, and a
  shared skill still ships as divergent copies.
- **Ship shared skills only in a core pack.** Rejected as a requirement: the
  plugin format has no plugin-to-plugin dependencies, so every person would
  have to install the core pack alongside a team pack. An organization may
  still choose to publish a core pack; the rules above make its skills and a
  team pack's copies identical.
- **Allow several versions of one package.** Rejected: two versions of a shared
  skill would emit one name with two bodies.
- **Version plugins with the marketplace package.** Rejected: every pin bump
  would re-version and rewrite every plugin in the marketplace.
- **Let a dependency's plugins ship automatically.** Rejected: a package's own
  plugins include work in progress and local test packs; the marketplace
  chooses what it publishes.
