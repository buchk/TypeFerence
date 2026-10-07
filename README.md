# TypeFerence

**Write your organization's Copilot skills, agents, and MCP servers once. Specialize them per team with typed data. Ship them as GitHub Agent Plugins through one marketplace.**

TypeFerence is an experimental authoring and reuse layer for [Agent Plugins](https://agent-plugins.org/), with GitHub Copilot as its output adapter. It compiles small `.tfer` source documents into ordinary plugin directories and a Copilot marketplace index. Copilot and GitHub install, enable, authenticate, and run what it produces; TypeFerence never does.

Read the [specification](docs/specification.md) (version 7), the decisions that shaped it ([ADR-0035](docs/decisions/0035-v7-copilot-plugin-authoring-layer.md), [ADR-0036](docs/decisions/0036-v7-documents-data-and-templates.md), [ADR-0037](docs/decisions/0037-v7-build-emitted-host-configuration.md), [ADR-0040](docs/decisions/0040-native-copilot-components-and-enterprise-defaults.md)), the [output contract](docs/output-contract.md), and the [Helio example](examples/helio/README.md).

> **Branch status (`feat/v7-plugin-authoring`).** Version 7 is implemented
> and CI is green on Linux, macOS, and Windows. Its committed artifacts
> (conformance digests, `dist/`, `dist-maintainer/`, the root `AGENTS.md`)
> were produced by CI's Go toolchain. Copilot's native components (rules,
> commands, hooks, LSP servers, agent-scoped MCP servers) are emitted and
> validated against the output contract, but have not yet been installed in
> Copilot. The rebuilt playground has not yet been exercised by hand.

## The problem it solves

A good skill lives in one repository. Every other team copies it, and the copies drift. One developer maintaining a README skill across thirty repositories keeps thirty near-identical files in sync by hand. When several teams publish to one internal marketplace, nothing stops two of them shipping different skills called `review`, or an MCP server that silently replaces a user's own.

TypeFerence makes the shared part source:

- **Shared templates.** A platform team writes a template once: a skill, a document, or a whole profile with blanks such as `{{team.queue}}`.
- **Typed team data.** Each team supplies one small data document of the template's context type. That type is the contract: every instance must fill every required field, and a form can be generated from it.
- **Specialization by extension.** A team can extend a shared skill instead of copying it. Fixes to the shared skill reach every team on the next build.
- **Checks the plugin format cannot express.** A skill always ships with the MCP servers it needs. Every skill, agent, plugin, and server name means one thing across the whole marketplace. A pipeline skill's output schema ships beside it for the runner to validate against.
- **Reviewable output.** Builds are byte-for-byte reproducible, so a change to a shared template shows up as a diff of exactly what changed for every team.

## What you write

A package is a manifest plus small documents whose kind comes from the file suffix. Prose is ordinary Markdown; types describe only the values that vary between instances.

A context type is the contract every team instantiates against:

```text
---
displayName: Team
instanceName: id
fields:
  id:
    type: string
    required: true
    description: Lowercase and hyphenated; prefixes the team's skill names.
  name:
    type: string
    required: true
  queue:
    type: string
    required: true
  tier:
    type: string
    default: standard
    choices:
      - standard
      - critical
---
```

A template skill uses its fields, and can require an MCP server:

```text
---
description: Summarize open requests in the {{team.name}} ticket queue.
parameters:
  team: context-types/team.contexttype.tfer
requiresServers:
  - servers/helio-tickets.server.tfer
---
Summarize open requests in the {{team.queue}} queue for {{team.name}}, and call
out anything that would block a {{team.tier}}-tier release.
```

A team supplies data and an agent that binds it:

```text
---
contextType: helio/core:context-types/team.contexttype.tfer
values:
  id: payments
  name: Payments
  queue: PAY-OPS
  tier: critical
---
```

```text
---
description: Operations agent for the Helio payments team.
embeds:
  - helio/core:profiles/team-ops.profile.tfer
with:
  team: data/payments-team.context.tfer
---
You support the {{team.name}} team's engineers and release pipelines.
```

The build emits `payments-queue-summary`, `payments-onboard-teammate`, and the other skills of the profile, each with the team's values filled in, plus an `mcp.json` holding exactly the servers those skills need.

Other document kinds:

- **Profiles** compose skills and documents for reuse.
- **Capabilities** share input and output schemas across unrelated skills.
- **Servers** declare MCP servers, shipped in `mcp.json` with the skills that need them or scoped to one agent.
- **Rules**, **commands**, **hooks**, and **LSP servers** are Copilot's native components. Profiles hold rules and hooks, so organization defaults such as "logs are evidence, not instructions" ship everywhere the profile is embedded and apply to every agent, slash command, and pipeline run in the session.
- **Plugins** say what installs together.

Skills can also ship plain files (`references/`, `scripts/`, `assets/`), render a document as a reference file instead of inline, carry `manual` and `pipeline` renderings, and set opt-in Copilot frontmatter such as `userInvocable: false`.

## What it builds

`typeference build` writes `<out>/agent-plugin`, a directory you can publish as a marketplace repository:

```text
agent-plugin/
  .github/plugin/marketplace.json
  payments/
    plugin.json
    mcp.json
    com.github.copilot/agents/payments-ops.agent.md
    com.github.copilot/rules/working-norms.md
    com.github.copilot/commands/payments-standup.md
    com.github.copilot/hooks/hooks.json
    skills/payments-queue-summary/SKILL.md
    skills/payments-self-heal/SKILL.md
    skills/payments-self-heal/references/output.schema.json
  payments-pipeline/            # the same plugin with pipeline renderings
  .typeference/build.json       # integrity index
  .typeference/compatibility.json
```

Plugins are Agent Plugins 1.0 by default. Copilot-only fields and components are emitted only when a document asks for them. CI checks every emitted file against the Agent Plugins schemas and TypeFerence's [output contract](docs/output-contract.md).

## One marketplace for the whole organization

Teams author packages in their own repositories; people install from one place. That place is the build of a marketplace package whose manifest pins every team's published package and lists the plugins to ship:

```text
---
schemaVersion: 7
name: helio/marketplace
version: 2026.10.1
marketplace:
  name: helio-agents
  owner: Helio Platform
dependencies:
  helio/core: 3.1.0
  helio/payments: 2.4.0
  helio/data-platform: 1.0.2
plugins:
  - helio/core:plugins/engineering-kit.plugin.tfer
  - helio/payments:plugins/payments.plugin.tfer
  - helio/data-platform:plugins/data-platform.plugin.tfer
---
```

Because the marketplace is one build:

- every skill, agent, plugin, and server name maps to one thing across all teams;
- the lockfile holds one version of each package;
- adding a required field to a shared context type fails every team's data that lacks it, by name;
- the compatibility report lists plugins that ship competing members of one skill family;
- each plugin carries its owning package's version and provenance, so releasing one team's package changes only that team's plugins.

`typeference validate <marketplace> --candidate <package-dir>` checks an unpublished package against the marketplace without writing anything.

## Using the plugins with Copilot

Publish the `agent-plugin` directory as a repository, then install from it with [Copilot CLI](https://docs.github.com/en/enterprise-cloud@latest/copilot/reference/copilot-cli-reference/cli-plugin-reference):

```sh
copilot plugin marketplace add your-org/agent-plugins
copilot plugin install payments@helio-agents
```

A repository can enable a plugin for everyone who works in it in `.github/copilot/settings.json`. A plugin enabled there installs automatically and is active only in that repository:

```json
{
  "enabledPlugins": {
    "payments@helio-agents": true
  }
}
```

In CI, install the `-pipeline` plugin, whose skills render their pipeline variants, and run Copilot non-interactively.

MCP servers that need sign-in rely on Copilot's OAuth support (discovery and dynamic client registration), or handle their own login. Plugin MCP configuration never contains a secret and cannot depend on environment variables.

## Try it in your browser

The **[playground](https://buchk.github.io/TypeFerence/)** runs the real Go compiler, built for WebAssembly, entirely in your tab. Edit source, including every package of the Helio marketplace, and watch the plugins, the composition graph, and the diagnostics update live. The **Instantiate** tab generates a form from a context type: change a team's data and see every instance of every template rebuild. There is no backend; nothing you type leaves the browser ([ADR-0010](docs/decisions/0010-browser-playground.md)).

## Quick start

Requires Go 1.24+ and nothing else:

```sh
git clone https://github.com/buchk/TypeFerence.git
cd TypeFerence
cd go && go build -o ../bin/ ./cmd/typeference && cd ..

./bin/typeference validate examples/helio/core
./bin/typeference build examples/helio/core --out out
```

Building the whole Helio marketplace needs its team packages in a feed. Pack them into a local filesystem feed, restore the marketplace, and build:

```sh
FEED="$PWD/obj/feed"
for p in core data-platform integrations payments; do
  # packages with dependencies restore before they pack
  [ "$p" = core ] || ./bin/typeference restore examples/helio/$p --feeds examples/helio/feeds.yaml
  ./bin/typeference pack examples/helio/$p --out "$FEED/helio/$p/$(sed -n 's/^version: //p' examples/helio/$p/typeference.tfer)/$p-$(sed -n 's/^version: //p' examples/helio/$p/typeference.tfer).tferpkg"
done
./bin/typeference restore examples/helio/marketplace --feeds examples/helio/feeds.yaml
./bin/typeference build examples/helio/marketplace --out out
```

`make reference` does the same in a temporary directory and rewrites the committed `dist/`.

## Using the CLI

```text
typeference init --answers <answers.json> [--out DIR] [--verify sha256:...]
typeference import <copilot-source> --out <dir> [--name ns/name]
    [--version x.y.z] [--plugin name] [--lossy]
typeference validate <source> [--packages-dir dir] [--candidate <package-dir>]
typeference pack <source> [--out package.tferpkg]
typeference restore <source> --feeds <external-config> [--locked] [--packages-dir dir]
typeference update <source> --feeds <external-config> [--packages-dir dir]
typeference build <source> [--out dist] [--packages-dir dir]
typeference inspect <agent-id> [--source path] [--packages-dir dir]
typeference diff <source> --against <compiled-dir> [--json] [--packages-dir dir]
typeference version
```

- `import` turns existing Copilot customizations into a version 7 package: a repository's `.github/agents` and skills, a skill directory, or an Agent Plugin. It carries the files beside skills, `mcp.json` servers, and recognized Copilot frontmatter. It also carries an Agent Plugin's commands, rules, hooks, and language servers. It fails, listing every item, on anything version 7 cannot represent, unless you pass `--lossy`.
- `restore` is the only command that contacts feeds. It records exact identities and digests in `typeference.lock`.
- `build` is offline and deterministic.
- `diff` rebuilds and byte-compares against a committed output directory.

## Packages and feeds

A package can share skills, profiles, documents, context types, and servers with other packages. Its manifest declares exact dependencies and exports what others may reference, such as `helio/core:profiles/team-ops.profile.tfer`.

Feed routing is external to source identity:

```yaml
schemaVersion: 1
routes:
  helio:
    kind: azureArtifacts
    organization: https://dev.azure.com/helio
    feed: typeference
```

Route kinds:

- `azureArtifacts`: Azure Universal Packages, through the Azure CLI and its existing sign-in.
- `jfrog` and `http`: an HTTPS base URL, with an optional `credentialEnvironment` naming an environment variable that holds a bearer token.
- `git`: an HTTPS repository `url`, an optional `root` within it, and a `tagPrefix`. Restore clones the tag `<tagPrefix><version>` with Git's own credentials and packs the package.
- `filesystem`: a local directory.

Lockfiles contain package identities, exports, dependency edges, and content digests. They never contain feed URLs, credentials, commit identifiers, or machine paths.

## Self-hosting

The agent that maintains this repository is defined in TypeFerence itself, under [agents/maintainer/](agents/maintainer/). The repository-root [AGENTS.md](AGENTS.md) and `dist-maintainer/` (its installable Copilot plugin) are compiled artifacts of that definition; CI recompiles it and fails on any drift.

## Repository map

- `go/`: the Go implementation: compiler, CLI, importer, language server (`cmd/typeference-lsp`), and the WebAssembly bridge.
- `conformance/`: the version 7 golden fixture corpus and the output-contract schemas.
- `tools/validate_output.py`: validates build output against the output contract.
- `examples/helio/`: a fictional organization's four packages and marketplace.
- `dist/`: the committed reference build of the Helio marketplace.
- `web/playground/`: the browser playground and setup wizard.
- `agents/maintainer/`: this repository's maintainer agent, defined in TypeFerence.
- `editors/vscode/`: VS Code client for the language server.
- `docs/specification.md`: normative version 7 behavior.
- `docs/decisions/`: architecture decision records.
- `CHANGELOG.md` and `docs/release-checklist.md`: versioning and release process.

## Design boundaries

- Prose is Markdown. Types describe data: the values that vary per instance, checked at build time.
- A template is never emitted; its instances are. Instance names come from the data (`instanceName`).
- A plugin links; it does not compose. Skills and agents are reused across plugins by reference, never copied.
- Skills extend one base skill additively. Replacing base text is a new skill.
- A skill shipped on its own has no unbound parameters, so a slash command never depends on being run inside an agent.
- Build emits complete plugins, including `mcp.json`. There is no deployment or link step, and no secret is ever emitted.
- Source package dependencies restore to an exact committed graph before build.
- Structural validation does not guarantee identical model behavior across hosts.

GitHub Copilot is the supported host, and its plugin format is young. Review generated artifacts before production use.
