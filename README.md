# TypeFerence

> **Version 7 in progress (branch `feat/v7-plugin-authoring`).** On this branch,
> TypeFerence is narrowed to an authoring and reuse layer for Copilot plugins.
> - Documents are free Markdown, data is typed, and context types are the
>   contracts teams instantiate shared templates against.
> - MCP servers and extra skill files are source, and build emits complete
>   plugins.
> - The neutral, ARD, A2A, link, trust, interface, and evaluation surfaces are
>   removed.
>
> Read [ADR-0035](docs/decisions/0035-v7-copilot-plugin-authoring-layer.md),
> [ADR-0036](docs/decisions/0036-v7-documents-data-and-templates.md),
> [ADR-0037](docs/decisions/0037-v7-build-emitted-host-configuration.md), the
> amended [specification](docs/specification.md), and the
> [Helio v7 example](examples/helio-v7/README.md).
>
> **The compiler, tests, and committed `dist/` still implement version 6.**
> The rest of this README describes version 6 until the implementation
> catches up.

**Define your organization's agents and skills once. Compose them with types. Ship them as GitHub Agent Plugins that people install once and use in every repository and pipeline.**

TypeFerence is an experimental reference implementation of a typed definition and compilation layer for AI agents. It replaces copied, drifting agent files with Go-like composition: reusable profiles, agent embedding, skills that extend shared skills, structurally satisfied interfaces, typed context, deterministic compilation, provenance, and artifact diffing. Its primary output is a marketplace of [Agent Plugins](https://agent-plugins.org/) for GitHub Copilot.

Read the [whitepaper](docs/whitepaper.md), the [rendered PDF](output/pdf/typeference-whitepaper.pdf), the [draft v6 specification](docs/specification.md), or the [ARD alignment notes](docs/ard-alignment.md). The design of version 6 is recorded in [ADR-0029](docs/decisions/0029-agent-plugins-primary-target.md) through [ADR-0033](docs/decisions/0033-import-copilot-customizations.md).

## The problem it solves

A well-tuned custom agent lives in one repository's `.github/agents/`. Using it anywhere else means copying it, and every copy drifts. Across an organization that becomes dozens of near-identical agents and skills, each team reinventing the last team's work, and a fix to the shared part reaches none of them.

TypeFerence makes the shared part a typed source package. Teams compose what they need from it, and the build emits installable plugins, so a team's agent and skills follow its people into every repository, into Copilot CLI and VS Code, and into CI runs.

## What you write

A version 6 package is a manifest and small `.tfer` documents. A document's kind comes from its file suffix and its identity from its path; nobody types an ID.

```text
examples/helio/
  typeference.tfer                          # package name, version, plugins, exports
  plugins/payments.plugin.tfer              # what ships together
  agents/payments-repo-agent.agent.tfer     # identity and objectives
  profiles/repository-defaults.profile.tfer # reusable composition
  skills/repository-status.skill.tfer       # a shared core skill
  skills/payments-repository-status.skill.tfer  # the payments team's extension of it
  context/payments-service.context.tfer     # typed context
```

A plugin links the agents, profiles, and skills that install together:

```text
---
description: The payments repository agent, for engineers and for release pipelines.
agents:
  - agents/payments-repo-agent.agent.tfer
modes:
  - manual
  - pipeline
---
```

An agent is thin, identity and objectives, and composes the rest:

```text
---
description: Specializes repository assistance for the fictional payments service.
embeds:
  - profiles/repository-defaults.profile.tfer
context:
  - context/payments-service.context.tfer
skills:
  - skills/payments-repository-status.skill.tfer
---
You report on the fictional payments service's repository for engineers and
for release pipelines. A healthy verdict requires every financial-control
signal.
```

The profile binds the shared `repository-status` skill; the agent's binding of the team's extension replaces it, and the extension adds its text to the core skill's instead of copying it:

```text
---
description: Report payments-service health with contract and reconciliation evidence.
extends: skills/repository-status.skill.tfer
requiresContextTypes:
  - context-types/payments-service.contexttype.tfer
---
For the payments service, also report payment-contract compatibility,
reconciliation checks, and rollback readiness.
```

(Simplified: the example's real extension also carries `pipeline` and `a2a` variants and a tool requirement.) When the core skill is fixed, every team's extension picks the fix up on its next build. A skill that should never be specialized is `sealed: true`.

## What it builds

`typeference build` writes a marketplace repository root:

```text
out/agent-plugin/
  .github/plugin/marketplace.json
  payments/                     # manual mode, for people
    plugin.json
    com.github.copilot/agents/payments-repo-agent.agent.md
    skills/payments-repository-status/SKILL.md
  payments-pipeline/            # pipeline mode, for CI
  engineering-kit/              # a skills-only pack
  executive/
  .typeference/compatibility.json
```

Skills stay `SKILL.md` files, so each one is still a slash command. A skill that needs its agent's context ships only with that agent; the compiler checks it. Names and descriptions are validated against the Agent Plugins and Agent Skills grammars, and every emitted skill name maps to exactly one skill across the build. `compatibility.json` reports plugins that ship competing members of one skill family, the case where installing both would let Copilot pick either.

It also writes the `neutral` bundle, the canonical host-independent form that link, A2A publication, and the ARD catalog build on.

## One marketplace for the whole organization

Teams author agents and skills in their own repositories, but people install from one place. That place is the build of a **marketplace package**: a small repository whose manifest pins every team's published package and lists the plugins to ship ([ADR-0034](docs/decisions/0034-organization-marketplace-from-source-packages.md)):

```text
---
schemaVersion: 6
name: acme/marketplace
version: 2026.10.1
marketplace:
  name: acme-agents
  owner: Acme Platform
dependencies:
  acme/core: 3.1.0
  acme/payments-agents: 2.4.0
  acme/data-agents: 1.0.2
plugins:
  - acme/core:plugins/engineering-kit.plugin.tfer
  - acme/payments-agents:plugins/payments.plugin.tfer
  - acme/data-agents:plugins/data.plugin.tfer
---
```

Because the whole marketplace is one build, the guarantees of a single build hold across every team:

- Every plugin, agent, and skill name maps to exactly one thing. If two teams ship different skills called `review`, the marketplace build fails and names both, instead of Copilot keeping whichever it loads first.
- A shared skill from a core package is one skill, emitted byte-for-byte identically in every plugin that carries it, so installing two packs that include it is harmless.
- The lockfile holds one version of each package. If payments pins `acme/core` 3.1.0 and data pins 3.2.0, restoring the marketplace fails until they agree.
- The compatibility report covers the whole marketplace: it names plugins that ship competing variants of one skill.
- Each plugin carries its owning package's version and provenance, so releasing one team's package changes only that team's plugin directories.

The flow:

1. A team merges, and its CI publishes a new version of its package to the feed with `typeference pack`.
2. A pull request bumps that package's pin in the marketplace package (`typeference update` rewrites the lockfile).
3. Marketplace CI builds it. Collisions, divergent shared skills, and version skew fail here.
4. A release job replaces the published marketplace repository's contents with the build output (linked, when plugins import tools). Nothing else goes in, so a plugin reaches people only by going through the compiler.

A team can check its package against the current marketplace before publishing:

```sh
typeference validate path/to/marketplace --candidate .
```

It reports collisions, the packages that must move with a core release, and competing plugins, and writes nothing.

## Using the plugins with Copilot

Publish the `agent-plugin` directory as a repository (here `your-org/agent-plugins`; the marketplace it defines is named `helio-agents` by the manifest), then install from it with [Copilot CLI](https://docs.github.com/en/enterprise-cloud@latest/copilot/reference/copilot-cli-reference/cli-plugin-reference):

```sh
copilot plugin marketplace add your-org/agent-plugins
copilot plugin install payments@helio-agents
```

A repository can enable a plugin for everyone who works in it in `.github/copilot/settings.json`; a plugin enabled there installs automatically and is active only in that repository:

```json
{
  "enabledPlugins": {
    "payments@helio-agents": true
  }
}
```

Marketplaces that are not added by default are declared in the same file under `extraKnownMarketplaces`; see GitHub's [repository settings reference](https://docs.github.com/en/enterprise-cloud@latest/copilot/reference/copilot-cli-reference/cli-config-dir-reference) for its format.

In CI, install the `-pipeline` plugin, whose skills render their pipeline variants, and run the agent non-interactively:

```yaml
permissions:
  contents: read
  copilot-requests: write
steps:
  - uses: actions/checkout@v6
  - run: npm install -g @github/copilot
  - run: |
      copilot plugin marketplace add your-org/agent-plugins
      copilot plugin install payments-pipeline@helio-agents
      copilot --agent payments-repo-agent -p "Report release readiness." -s
    env:
      GITHUB_TOKEN: ${{ github.token }}
```

A job's default `GITHUB_TOKEN` can read only the job's own repository, so installing from a private marketplace in another repository needs a credential with read access to it; how Copilot CLI picks that credential up is still to be verified.

These host behaviors come from GitHub's documentation; the ones TypeFerence has not yet verified in practice are listed in [ADR-0029](docs/decisions/0029-agent-plugins-primary-target.md#consequences).

## Why not just write agent files?

You can, and for one agent in one repository you often should. TypeFerence becomes useful when agents and skills need reuse, review, specialization, provenance, and repeatable output across many repositories and teams.

Agent runtime system prompts are like machine code: they are what the model consumes at execution time. `.agent.md` and `SKILL.md` files are like assembly language: readable and controllable, but close to one host's concrete shape. TypeFerence is the higher-level language above them. Teams model profiles, skills, capabilities, context, and trust metadata once, and the compiler emits the files.

Already have agents and skills? `typeference import` turns a repository's `.github/agents` and skills, a skill directory, or an existing plugin into a version 6 package. It fails, listing every item, when the source holds something version 6 cannot represent (tool restrictions, MCP configuration, scripts beside a skill), unless you pass `--lossy`.

## Where it fits

```text
declared source packages
    -> restore exact locked dependency tree
    -> build deterministic unlinked target artifacts
    -> link explicit deployment bindings
    -> publish or run through the selected host
```

[Agentic Resource Discovery](https://agenticresourcediscovery.org/) helps clients find and verify deployed capabilities. TypeFerence addresses the earlier authoring problem: producing compatible native artifacts from one governed definition. Discovery portability does not itself provide definition portability.

The long-term objective is behavioral equivalence: preserving declared organizational intent across the places an agent runs closely enough to be measured and governed. Version 6 provides the closed typed source, deterministic output, and provenance needed to test that objective; it does not claim that different models or runtimes already behave identically.

## Try it in your browser

The **[playground](https://buchk.github.io/TypeFerence/)** runs the real Go
compiler — built for WebAssembly, internals untouched — entirely in your tab.
Edit typed source and watch the compiled plugin and neutral artifacts, the
composition graph, and the diagnostics update live. The status-bar digest is
the determinism guarantee made interactive: the Helio example reproduces the
digest of this repository's committed `dist/` exactly, and the self-hosting
example recompiles the repository's own root `AGENTS.md` byte for byte. There
is no backend; nothing you type leaves the browser
([ADR-0010](docs/decisions/0010-browser-playground.md)).

The playground's **Equivalence** tab walks the full BETH loop (ADR-0009)
without ever asking for a credential: it packs scenario × surface cells with
the real `equivalence pack` code, you collect responses by copy/paste from
real hosts (BETH's operator model), it exports the assembled run as a
deterministic `.tar.gz`, you score locally — keys stay in your terminal — and
dropping the resulting `scorecard.json` back onto the page renders adherence,
agreement, and every divergence
([ADR-0011](docs/decisions/0011-playground-live-runs.md)).

## Quick start

Requires Go 1.24+ and nothing else. From clone to compiled artifacts:

```sh
git clone https://github.com/buchk/TypeFerence.git
cd TypeFerence
cd go && go build -o ../bin/ ./cmd/typeference && cd ..

./bin/typeference validate examples/helio
./bin/typeference build examples/helio --out out
./bin/typeference link out/agent-plugin --deployment examples/deployment/helio-plugins.yaml --out linked/plugins
./bin/typeference link out/neutral --deployment examples/deployment/helio-a2a.yaml --out linked/a2a
./bin/typeference diff examples/helio --against dist
./bin/typeference inspect helio/works/agents/payments-repo-agent@1.0.0 --source examples/helio
```

`build` writes deterministic, unlinked `agent-plugin` and `neutral` targets
under `out/`, plus an ARD catalog because the example's manifest declares a
publisher. It emits no MCP configuration, command, or endpoint. `link`
validates an explicit external deployment file and materializes runtime
configuration: for plugins, a credential-free `mcp.json` per plugin. It creates
an absent or empty output and replaces a non-empty one only when root link
provenance identifies it as a prior TypeFerence output. `diff` recompiles and
byte-compares against the committed reference output in `dist/` — "No
differences." is the determinism guarantee made visible: your freshly built
compiler reproduces the repository's artifacts exactly.

The binary is fully static (`CGO_ENABLED=0`) with no runtime dependencies. You can
also install it with Go directly (requires the module to be published on the
repository's default branch):

```sh
go install github.com/buchk/TypeFerence/go/cmd/typeference@latest
```

Tagged releases ship prebuilt archives for Linux, macOS, and Windows (amd64/arm64)
with a `SHA256SUMS` file — unpack one binary and put it on `PATH`; there is no
installer to run. The release process is documented in
[docs/release-checklist.md](docs/release-checklist.md).

## Using the CLI

If you downloaded a release archive instead of building from source, unpack
`typeference` (`typeference.exe` on Windows) onto `PATH` and confirm it runs:

```sh
typeference version
```

`<source>` in every command below is a version 6 package: a directory with a
`typeference.tfer` manifest. `typeference init` scaffolds one from a setup
wizard answer set, and `typeference import` creates one from existing Copilot
customizations.

```text
typeference init --answers <answers.json> [--out DIR] [--verify sha256:...]
typeference import <copilot-source> --out <dir> [--name ns/name]
    [--version x.y.z] [--plugin name] [--lossy]
typeference validate <source> [--trust-config path]
    [--packages-dir obj/typeference/packages] [--candidate <package-dir>]
typeference pack <source> [--out package.tferpkg]
typeference restore <source> --feeds <external-config> [--locked]
    [--packages-dir obj/typeference/packages]
typeference update <source> --feeds <external-config>
typeference build <source> [--target all|agent-plugin|neutral] [--out dist]
    [--packages-dir obj/typeference/packages]
    [--emit-ard --publisher-domain example.com] [--trust-config path]
    [--trust-signatures signatures.json] [--allow-unsigned-trust]
typeference inspect <agent-id> [--source path]
typeference link <built-target-dir> --deployment <file> --out <linked-dir>
typeference diff <source> --against <compiled-dir> [--target all]
    [--emit-ard --publisher-domain example.com] [--trust-config path]
    [--trust-signatures signatures.json] [--json] [--allow-unsigned-trust]
typeference eval <source> --scenarios <file-or-dir> [--live] [--model id] [--out dir]
typeference equivalence pack <source> --scenarios <file-or-dir> --out <run-dir>
    [--target all|<name>[,<name>...]]
typeference equivalence score <run-dir> [--live] [--model id]
```

`validate` checks composition, typing, and every plugin's shipping rules
without writing anything; with `--candidate`, it checks an unpublished package
against a marketplace package's locked graph. `pack` creates a canonical source package. `restore`
is the only dependency operation that contacts feeds; it commits exact
identities and digests to `typeference.lock` and materializes the complete tree
under `obj/typeference/packages`. Once a lock exists, `restore` honors it;
`update` is the explicit re-resolution after exact manifest edits. `build` is
offline and frozen. `link` is the separate environment-specific transform.
`diff` recompiles and byte-compares against an already-built directory — see
[Quick start](#quick-start) above for what "No differences." means. `eval` and
`equivalence` are covered in [Behavioral evals](#behavioral-evals) below; both
default to a dry run that never makes a network call.

The language server (`go/cmd/typeference-lsp`) and its
[VS Code client](editors/vscode/README.md) give `.tfer` authors diagnostics,
completions, and go-to-definition on reference paths.

## Source format, composition, and exposure

Every document is a `.tfer` file: frontmatter in TypeFerence's own closed
grammar between `---` fences, then a body. A skill's body is its instructions,
a context's body is its text, and an agent's body is its objectives. Plain
text needs no quotes; each field's declared type decides what a value means,
and nothing is guessed from its spelling. The grammar is normative in the
[specification](docs/specification.md#frontmatter-grammar) and
[ADR-0032](docs/decisions/0032-v6-grammar-and-schema-directed-scalars.md).

Composition is Go-like. Agents embed profiles or other agents, profiles embed
profiles, and the shallowest binding of a capability wins, so a team agent can
embed a shared agent and change one skill, or change nothing and just bundle
job-shaped profiles. A skill that binds no capability defines its own; binding
a skill whose capability an embedded layer already binds replaces it, unless
that binding is `sealed`. Interfaces are satisfied structurally by the resolved
slots and capability bindings of an agent.

`required` is the demand side of the composition model. A profile may declare a
required capability without choosing its implementation; every concrete agent
that carries the requirement must supply a compatible binding. Interface
satisfaction is observational—it reports what an agent already provides—and does
not create that obligation. `sealed` is separate: it protects an implementation
that has already been supplied.

Capability visibility is a separate public-API axis: `internal` is the default,
and both internal and exposed capabilities can satisfy interfaces and
participate in composition. Only `visibility: exposed` capability documents are
projected onto linked callable surfaces such as A2A Agent Cards.

Context governance uses a different boundary. A context satisfies its declared
type and only the base types it explicitly refines through `embeds`; a
structurally identical but unrelated type cannot impersonate a governed type.
A context with no declared type is plain text. Tools are independent extern
dependencies consumed by skills, not alternate implementations of those
skills' capabilities. These boundaries are recorded in
[ADR-0024](docs/decisions/0024-clarify-v4-type-and-composition-boundaries.md).

## One implementation, one specification

The Go implementation under `go/` is the reference implementation. The
[specification](docs/specification.md) stays normative in principle — the
abstraction is the contract, and anyone may realize it differently — but this
repository's Go compiler is the one living answer ([ADR-0014](docs/decisions/0014-go-only-implementation.md)).
Determinism is preserved and made visible by the
[conformance suite](conformance/README.md): the compiler must reproduce the
committed digests byte-for-byte.
The [specification evidence matrix](docs/conformance-matrix.md) maps normative
areas to golden fixtures and focused implementation tests. Architectural
follow-ups are maintained in [next steps](docs/next-steps.md).

The repository's executable corpora have distinct jobs: `examples/helio` is the
integrated narrative and committed reference output; `agents/maintainer` is the
self-hosting drift gate; and `conformance/fixtures` is the normative edge-case
corpus. New features should not be copied into every example unless that role
requires them.

An earlier C# reference implementation was retired when the project committed to
being a tool rather than a multi-implementation standard. Determinism — the
property that makes `diff` and the committed-digest guarantee real — is a property
of one good compiler and was unaffected; only the second implementation, and the
cross-implementation half of the conformance suite, went away. TypeFerence embeds
no LLM provider. Deployment state is consumed only by the explicit linker and
never changes source or unlinked-target identity.

## Packages and enterprise feeds

A package can share agents, profiles, and skills with other packages. Its
manifest declares exact dependencies and exports the documents others may use:

```text
---
schemaVersion: 6
name: helio/payments-agents
version: 1.0.0
dependencies:
  helio/foundations: 3.1.0
plugins:
  - plugins/payments.plugin.tfer
---
```

A document then references a dependency's export by package-qualified path,
such as `helio/foundations:skills/repository-status.skill.tfer`.

Feed routing is external to source identity. A filesystem/JFrog-compatible
example looks like:

```yaml
schemaVersion: 1
routes:
  helio:
    kind: jfrog
    baseUrl: https://artifacts.example/artifactory/typeference
    credentialEnvironment: JFROG_ACCESS_TOKEN
```

`azureArtifacts` routes use Azure Universal Packages through the Azure CLI and
its existing authentication. Package names replace `/` with `--` in Azure
Artifacts. Lockfiles contain package identities, exports, dependency edges, and
content digests—never feed URLs, credentials, or machine paths.

## Self-hosting

The agent that maintains this repository is defined in TypeFerence itself, under
[agents/maintainer/](agents/maintainer/). The repository-root
[AGENTS.md](AGENTS.md) and `dist-maintainer/` (its neutral bundle, an
installable Copilot plugin, and an ARD catalog) are compiled artifacts of that
definition; CI recompiles it and fails on any drift. What the exercise revealed
about the type system's limits is recorded honestly in
[ADR-0006](docs/decisions/0006-self-hosting-design-feedback.md).

## Behavioral evals

`typeference eval` runs scenario files (task prompt plus expected-behavior rubric)
against a compiled definition and scores rubric adherence with an LLM judge — dry
run by default, emitting the exact request payloads without any network call. A
pass is an adherence signal, not behavioral equivalence; the framing and its limits
are documented in [evals/README.md](evals/README.md).

`typeference equivalence` (BETH, the Behavioral Equivalence Test Harness) is the
deployment-side counterpart: `pack` lays out the same scenarios as run-ready cells
per compiled target surface, an operator collects one response per cell from a real
host, and `score` reports adherence per surface and agreement across surfaces,
listing every divergence. A scorecard is one observation per surface, not a proof;
see [ADR-0009](docs/decisions/0009-behavioral-equivalence-harness.md).

`--emit-ard` (the default when the manifest declares a `publisher`) emits one
canonical TypeFerence source-package entry and one unlinked bundle entry for
each neutral agent bundle and plugin artifact. Entries carry `derivedFrom`
provenance back to the canonical explicit source-resource digest. Callable MCP
or A2A publication requires linked provider/endpoint metadata; build never
invents an address.

### Trust manifests

An optional `typeference.trust.tfer` at the source root enriches the draft AI Catalog `trustManifest` for the source package and compiled bundles. It can declare DID, SPIFFE, or HTTPS identity; trust schemas; attestation and provenance references; policy or enterprise verification metadata; and an intent to sign. TypeFerence validates and publishes these declarations without resolving remote documents or asserting that an external authority has verified them.

```text
---
schemaVersion: 5
source:
  identity: did:web:helio.example:typeference:source:helio
  identityType: did
  attestations:
    - type: https://slsa.dev/provenance/v1
      uri: https://trust.helio.example/provenance/source.intoto.jsonl
      digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
bundles:
  identityTemplate: spiffe://helio.example/typeference/{target}/{agent}
  identityType: spiffe
  metadata:
    com.helio.governance:
      policyDigest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      runtimeEvidenceProfile: tag:agentrust.io,2026:trace-v0.1
  signatureIntent:
    algorithm: ES256
    keyRef: did:web:helio.example#catalog-signing
---
```

TypeFerence does not hold signing keys. An external signer can produce detached JWS values over the unsigned, JCS-canonicalized trust manifests and provide them through `--trust-signatures signatures.json`. Use `--allow-unsigned-trust` only to emit signing input when `signatureIntent.required` is true; normal publication fails closed without those signatures. The signature map must be outside the source root so adding signatures cannot change the source digest they sign. Identical source, trust config, and signature map inputs produce byte-identical output.

## Repository map

- `go/` - the Go implementation: compiler, CLI, importer, language server (`cmd/typeference-lsp`), and eval harness.
- `conformance/` - canonical conformance fixtures (byte-identity contract).
- `examples/helio/` - fictional organization: a plugin marketplace with a multi-skill agent plugin, a skills pack, skill extension, and a pipeline plugin.
- `examples/deployment/` - deployment files for linking the Helio output.
- `web/playground/` - browser playground: the Go compiler built for `js/wasm`, plus the setup wizard and the BETH operator console.
- `agents/maintainer/` - this repository's maintainer agent, defined in TypeFerence.
- `editors/vscode/` - VS Code client for the language server.
- `evals/` - behavioral eval scenarios and honest framing.
- `docs/specification.md` - normative version 6 behavior.
- `docs/decisions/` - architecture decision records.
- `docs/whitepaper.md` and `output/pdf/typeference-whitepaper.pdf` - design paper.
- `CHANGELOG.md` and `docs/release-checklist.md` - versioning and release process.

## Design boundaries

- Agents may embed multiple profiles or agents; profiles may embed other profiles; local slots and capability bindings resolve promoted-name ambiguity.
- A plugin links; it does not compose. Agents and skills are reused across plugins by reference, never copied.
- Skills extend one base skill additively; a sealed skill cannot be extended, and an extension cannot change its base's contract.
- Interfaces may embed interfaces and are satisfied structurally, without declarations on agents.
- Context is a first-class typed resource. Raw prose is a context with a text
  body; there is no filesystem-path escape hatch. Refinement is nominal through
  explicit `embeds`, with structurally checked members.
- Tools are independent declared runtime imports used by skills. Base and variant
  requirements remain distinct until link selects deployment modes and providers.
- Build emits deterministic unlinked artifacts with provenance; link
  materializes active runtime configuration, and never a secret.
- Source package dependencies restore to an exact committed graph before build.
- No deployment state, hosted runtime, or model credentials participate in
  source or build semantics.
- Structural validation does not guarantee identical LLM behavior across models or hosts.
- ARD publication wraps selected target outputs; it is not itself a compilation target or execution runtime.

GitHub Copilot is the supported host, and its plugin format is young. Review generated artifacts before production use.

TypeFerence is licensed under Apache-2.0. Helio Works is fictional.
