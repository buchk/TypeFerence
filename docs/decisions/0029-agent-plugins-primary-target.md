# 0029 — GitHub Agent Plugins as the primary output target

## Status

Proposed (2026-09-29). On acceptance: supersedes ADR-0027 decision 3 for
`description`; retires the Codex, Copilot, and Cursor adapters from the
build-target list in the specification. Companion records: ADR-0030 (plugin
documents and v6 authoring), ADR-0031 (additive skill extension), ADR-0032 (the
v6 grammar), and ADR-0033 (import).

## Context

The existing `copilot` target emits repository-local files
(`.github/agents/<agent>.agent.md`, `.github/copilot-instructions.md`). A
well-tuned agent therefore works only in the repository it was copied into, and
nothing distributes it: an organization that wants the same agents and skills
in every repository, and in automated tasks, rebuilds them per repository.

Agent Plugins 1.0 (agent-plugins.org) is the packaging format GitHub Copilot
installs. A plugin is a directory with a closed `plugin.json`, portable
`skills/<name>/SKILL.md` components, an optional `mcp.json`, and client
extension namespaces; Copilot reads agents, hooks, commands, rules, and LSP
configuration from `com.github.copilot/`. Copilot CLI installs plugins into the
user's Copilot home, VS Code discovers those installs, and marketplaces
(`.github/plugin/marketplace.json`) distribute them. Repository settings
(`.github/copilot/settings.json`: `extraKnownMarketplaces`, `enabledPlugins`)
and enterprise-managed settings enable them; a plugin enabled only by a
repository's `enabledPlugins` auto-installs and activates in that repository
and stays disabled elsewhere, and Copilot cloud agent reads the same keys.
Copilot CLI also runs non-interactively (`copilot --agent <name> -p …`) in CI
with the job token.

The target audience for this direction is an organization standardized on
GitHub Copilot that wants shared agents and skills everywhere its people use
it. Other hosts are out of scope; the retired adapters have no consumers.

Three host facts shape the decision:

- Installed skills form one pool keyed by the SKILL.md `name`; duplicates
  resolve first-found-wins, and a repository's own skills and agents take
  precedence over plugin ones. A skill is invocable directly as a slash
  command.
- Custom agent profiles have no property that attaches skills to an agent
  (documented properties: `name`, `description`, `target`, `tools`, `model`,
  `disable-model-invocation`, `user-invocable`, `mcp-servers`, `metadata`).
- Skill and agent `description` values are how the host decides what to invoke.

## Decision

1. **`agent-plugin` is a build target.** `typeference build` emits it and
   `neutral` by default (`--target all`). The target root is a marketplace
   repository root: one directory per plugin artifact, the build index, the
   compatibility report (ADR-0031), and `.github/plugin/marketplace.json`,
   which lists each artifact by relative source. The marketplace index is
   emitted only when the project manifest declares the name and owner the
   format requires; build never invents them.
2. **Plugin layout.** Each artifact directory contains:
   - `plugin.json` with exactly the Agent Plugins 1.0 `$schema`, `name`,
     `version` (from the project manifest), and `description`;
   - `com.github.copilot/agents/<agent>.agent.md` for each linked agent:
     frontmatter `name` and `description`; a body of the agent's title,
     objectives, held context, and the emitted names of the skills it binds;
   - `skills/<skill>/SKILL.md` for every skill the plugin ships (ADR-0030),
     with extension chains flattened (ADR-0031) and skill-held context;
   - `.typeference/bundle.json` and `.typeference/link.json`, so an installed
     plugin traces to its source digest and states what link must bind.

   Build never emits `mcp.json`; link does (decision 7).
3. **Skills stay SKILL.md files.** Skills are not inlined into agent files:
   inlining would remove direct slash-command invocation and duplicate shared
   skills per agent.
4. **Names fail closed.** Plugin, agent, and skill names must satisfy the Agent
   Plugins and Agent Skills grammars, and descriptions their length limit;
   violations are errors, never rewrites. Across a build, each emitted skill
   name denotes one skill, each custom agent name one agent, and each artifact
   name one plugin mode.
5. **One mode per artifact.** A plugin's `modes` (default `manual`) select its
   artifacts: `manual` emits `<plugin>`, `pipeline` emits `<plugin>-pipeline`
   for CI installs. Each artifact renders exactly one mode, as a fixed-mode
   output in the sense of ADR-0024. A deployment selects a plugin's modes
   together and must select exactly the modes the plugin was built in, so no
   published marketplace lists an unlinked artifact. `a2a` remains a
   neutral-target and link concern.
6. **`description` is routing metadata (supersedes ADR-0027 decision 3).**
   Skill, agent, and plugin descriptions are required, single-line,
   length-bounded strings that emitters place only in routing surfaces:
   SKILL.md and custom agent frontmatter, `plugin.json`, marketplace entries,
   bundle and publication metadata, and the neutral bundle's skill index, which
   is how a neutral host chooses a skill. They never enter an instruction body;
   the neutral `AGENTS.md` no longer renders the agent's description. The
   field-classification rule gains this class alongside `displayName`, which
   titles instructions. This also resolves a present divergence: v5 emitters
   wrote the agent description into instruction bodies, which ADR-0027
   forbade.
7. **Tools through link, never secrets.** Link writes a credential-free
   `mcp.json` for each plugin artifact from the deployment file, with
   `{bundle}` expanded to `${PLUGIN_ROOT}/.typeference/bundle.json`. A stdio
   command must be a bare executable name or a plugin-relative `./` path.
   Agent Plugins forbids depending on inherited environment variables, so a
   provider that forwards environment variables or authenticates with a bearer
   token from the environment fails closed. Host-native configuration for
   credentialed CI use is specified after a pilot verifies Copilot behavior.
8. **Retire the Codex, Copilot, and Cursor adapters; keep `neutral`.** The
   neutral bundle remains the canonical all-modes artifact that link, A2A, and
   ARD publication build on. Removing the adapters removes their digests from
   the conformance manifests in the same change as the specification
   amendment. The archival corpora must reproduce their recorded neutral
   output byte-for-byte; version 6 neutral output renders objectives instead
   of the agent description, as decision 6 requires.
9. **Distribution stays outside build.** Where the marketplace repository
   lives, publishing to it (release automation writing a generated
   repository), repository settings, and enterprise settings are deployment
   concerns. Build emits no settings snippets: a marketplace's address is a
   deployment fact, and emitting one would put deployment metadata in an
   unlinked target.

## Consequences

- One source tree produces an installable marketplace; a tuned agent follows
  its user into every repository.
- Copilot is the only supported host. The neutral bundle keeps the definition
  layer host-independent without maintaining adapters nobody uses.
- The pool semantics make skill identity global: ADR-0030 and ADR-0031 define
  which skills a plugin ships and guarantee one body per emitted name per mode.
- Typed context field values reach Copilot only through the bundle; the
  Markdown a host reads carries each context's title and text body.
- GitHub's documentation settles two earlier questions: a repository's
  `enabledPlugins` both auto-installs a plugin and scopes it to that
  repository, and `extraKnownMarketplaces` is a map of named entries that each
  require a `source`. The following host behaviors are assumed but unverified,
  and must be checked in a pilot before this record is accepted: whether
  auto-install happens in non-interactive (`-p`) runs; the exact `source`
  object for a GitHub-hosted marketplace; whether plugin agents behave
  identically to repository agents; stdio MCP environment inheritance;
  `--allow-tool` syntax for MCP tools; how Copilot compares plugin versions
  for updates.

## Alternatives considered

- **Keep the repository-local `copilot` target as primary.** It cannot follow
  a user across repositories or be distributed; that is the problem.
- **One plugin per agent.** Rejected: the distribution unit is a curated bundle
  of agents and skills (ADR-0030).
- **Inline skills into agent files** so inheritance and overrides stay private
  to each agent. Rejected: loses slash-command invocation and multiplies
  near-identical skill text.
- **Link-time mode selection inside one plugin.** Rejected: separate
  `-pipeline` plugins are simpler to install and fit the existing fixed-mode
  adapter rule without amendment.
- **Keep the retired adapters frozen.** Rejected: every fixture would carry
  digests for targets with no consumers.
- **Emit repository-settings snippets from build.** Rejected: the snippet
  names the marketplace repository, which is deployment, not source.

## References

- Agent Plugins 1.0 specification and 1.1 draft: https://github.com/agentplugins/agent-plugins-spec
- Copilot CLI plugin reference: https://docs.github.com/en/enterprise-cloud@latest/copilot/reference/copilot-cli-reference/cli-plugin-reference
- Copilot CLI configuration directory (repository settings): https://docs.github.com/en/enterprise-cloud@latest/copilot/reference/copilot-cli-reference/cli-config-dir-reference
- Custom agents configuration: https://docs.github.com/en/copilot/reference/custom-agents-configuration
- Copilot CLI in GitHub Actions: https://docs.github.com/en/copilot/how-tos/copilot-cli/use-copilot-cli-in-actions
