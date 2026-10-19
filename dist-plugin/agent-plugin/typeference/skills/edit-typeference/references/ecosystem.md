# Packages, feeds, and marketplaces — quick reference

The specification is normative:
https://github.com/buchk/TypeFerence/blob/main/docs/specification.md
("Packages, restore, and lockfiles" and "Organization marketplaces").

## The pipeline

1. **restore** resolves the manifest's exact dependencies from feeds, verifies
   every package digest, writes `typeference.lock`, and materializes the graph
   under `obj/typeference/packages`. It is the only command that contacts a
   feed. With an existing lockfile it never rewrites it; `--locked` requires
   one.
2. **update** re-resolves and rewrites the lock after you change a dependency
   version in the manifest.
3. **build** reads only the source, the lockfile, and the materialized
   packages. It never contacts a network.
4. **publish** (outside TypeFerence) places output where hosts read it.

`pack` writes a canonical `.tferpkg` for a feed. Versions are exact; there is
no range solving, and a graph holds one version of each package.

## Sharing between packages

- The provider lists shareable documents in `exports` and shareable plugins in
  `plugins`.
- The consumer declares `dependencies: { <package>: <exact version> }` and
  references `<package>:<path>`.
- A personal or team plugin can embed another package's plugin and list only
  the extension it changes; a pin bump plus a rebuild picks up the provider's
  later fixes.

## Feed configuration

Feed routes are deployment, never source: keep the file outside the package
and pass its path to restore or update.

```yaml
schemaVersion: 1
routes:
  acme:
    kind: azureArtifacts
    organization: https://dev.azure.com/acme
    feed: typeference
```

Route kinds:

- `azureArtifacts`: Azure Universal Packages, through the Azure CLI sign-in.
- `jfrog`, `http`: an HTTPS base URL, with an optional
  `credentialEnvironment` naming an environment variable holding a token.
- `git`: an HTTPS repository `url`, optional `root`, and `tagPrefix`; restore
  fetches the tag `<tagPrefix><version>` with Git's own credentials.
- `filesystem`: a local directory.

A route selects one feed per namespace; nothing searches every feed.
Lockfiles never contain feed URLs, credentials, or machine paths.

## An organization marketplace

One package builds the marketplace. Its manifest declares `marketplace` and
lists the plugins of its dependencies:

```text
---
schemaVersion: 9
name: acme/marketplace
version: 2026.10.1
marketplace:
  name: acme-agents
  owner: Acme Platform
  files:
    - files/marketplace/README.md
dependencies:
  acme/core: 3.1.0
  acme/payments-agents: 2.4.0
plugins:
  - acme/core:plugins/engineering-kit.plugin.tfer
  - acme/payments-agents:plugins/payments.plugin.tfer
---
```

Because it is one build, every name means one thing across all teams, a
shared skill is emitted identically in every plugin that ships it, and the
compatibility report (`.typeference/compatibility.json`) lists plugins that
ship competing implementations of one capability.

Workflow for a team change:

1. The team validates its package, then checks it against the marketplace
   before publishing: `typeference validate <marketplace> --candidate <team
   package>`.
2. The team packs and publishes the new version to its feed.
3. The marketplace package bumps the pin, runs update, builds, and publishes
   the `agent-plugin` output as the marketplace repository's whole content.
   A README or CI workflow that repository needs is a `marketplace.files`
   entry, so it arrives through build too.

## Installing from a marketplace in Copilot

People add the marketplace repository and install plugins from it, for
example `copilot plugin marketplace add OWNER/REPO` then
`copilot plugin install <plugin>@<marketplace>`. Which repositories enable
which plugins (`.github/copilot/settings.json`) is deployment; build never
writes it.
