# 0004 — Source packages and one marketplace build

## Status

Accepted (2026-10-07).

## Context

Teams author agents and skills in their own repositories; people install
from one place. If each repository builds its own plugins and copies them into
the marketplace, collisions and version skew show up only as "these copies
differ", after the fact, and nothing can rebuild the marketplace from source.

## Decision

1. **Identity is source; address is deployment.** Package names, versions,
   paths, dependencies, the lock graph, and emitted content are source. Feed
   addresses, credentials, which repositories enable which plugins, and where
   the marketplace lives are deployment, and never appear in a manifest,
   lockfile, or digest. A plugin's descriptive `homepage` and `repository`
   are source: nothing retrieves from them.
2. **Restore, then build.** Manifests declare exact dependency versions.
   `pack` makes a canonical package, `restore` resolves through configured
   routes (filesystem, HTTP, JFrog, Azure Artifacts, Git tags; one namespace,
   one route, no fallback), verifies digests, and writes a lockfile. Build is
   offline and never restores.
3. **The marketplace is one build.** A marketplace package pins team packages
   and lists the plugins to ship as `<package>:<path>`. Every rule of a build
   then spans the whole marketplace: one name, one thing; one version of each
   package; shared skills emitted identically; one compatibility report.
   `validate --candidate` checks an unpublished package against it.
4. **Artifacts belong to their owning package.** A plugin carries its owning
   package's version and provenance, so a team release changes only that
   team's plugins and the index files.
5. **Only compiler output reaches the marketplace repository.** Publication
   replaces the repository's contents with the target, so anything else is
   drift. A README, license, or CI workflow the repository needs is a
   `marketplace.files` entry, and a plugin's README or adoption templates are
   its own `files`. Both go through build, and `build.json` lists every root
   file with a digest. Build never emits repository settings
   (`.github/copilot/`).

## Consequences

- Collisions, divergent shared skills, and version skew fail in a pull request
  against the marketplace package, not on someone's machine.
- Every team release is a reviewed pin change, and shared core packages move
  in lockstep. The candidate check shows which dependents must move.
- The whole marketplace builds with one compiler version.

## Alternatives considered

- **Collect built plugins and merge them.** Checks would run after the fact,
  and a merge path is exactly the side door the marketplace must not have.
- **Qualify emitted names by package.** Slash commands get worse for everyone,
  and shared skills still ship as divergent copies.
- **Several versions of one package.** One skill name would have two bodies.
- **Version plugins with the marketplace.** Every pin bump would rewrite
  every plugin.
- **Range solving.** Deferred until exact pins become a real burden.
- **A publishing overlay outside build for README and CI.** The repository
  would no longer be compiler output alone, and the overlay would have no
  digest.
