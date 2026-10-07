# 0007 — Build emits complete Copilot configuration

## Status

Accepted (2026-10-07).

## Context

A plugin is only useful if it installs and works without a later step. The
plugin format cannot say that a skill needs a server, and Copilot applies some
rules silently: a misnamed hook event is ignored, an agent's tool allowlist
hides tools it does not name, a plugin server called `github` replaces a
user's own. Organization rules must reach every surface (slash commands,
model-invoked skills, agents, pipelines), not only agent files.

## Decision

1. **Complete artifacts, no link phase.** Build writes `plugin.json`,
   `mcp.json`, skills, agents, rules, commands, hooks, and LSP configuration.
   `.server.tfer` documents declare MCP servers; skills list them in
   `requiresServers`, and an artifact's `mcp.json` holds exactly the servers
   its skills need in its mode.
2. **Servers are namespaced and secret-free.** Names have at least two
   hyphenated segments, commands are bare or plugin-relative, URLs are HTTPS,
   and no value may contain `${` other than the plugin variables. One server
   name denotes one configuration across the marketplace.
3. **Copilot-only fields are explicit.** A `copilot` mapping holds skill and
   agent frontmatter Copilot reads beyond Agent Skills. Nothing is inferred:
   `allowed-tools` is never derived from server requirements, and an agent
   that declares `tools` must list every server its skills need; build fails
   on a gap and never adds a grant. `tools: []` means no tools.
4. **Native components.** Rules, commands, hooks, and LSP servers are document
   kinds emitted into `com.github.copilot/`. Enterprise defaults are rules
   held by profiles, so they apply wherever the plugin is active. Build checks
   what Copilot would otherwise ignore silently (hook events and matchers,
   prompt-hook placement, HTTPS hooks, LSP launch fields).
5. **Plugin metadata.** A plugin's `author`, `homepage`, `repository`,
   `license`, and `keywords` are emitted into `plugin.json` and its
   marketplace entry only when declared. Its `category` and `tags`, and the
   marketplace's `description`, are emitted only into the marketplace index:
   Copilot documents them there, and Agent Plugins 1.0 does not define them
   in `plugin.json`.
6. **The output contract is tested.** `docs/output-contract.md` lists every
   emitted file; CI validates them against the vendored Agent Plugins schemas
   and TypeFerence's schemas for Copilot formats that GitHub does not publish.

## Consequences

- "This skill needs that server" and "this agent can use that server" are
  checked at build time.
- Credentials stay out of TypeFerence; servers that need sign-in use
  Copilot's OAuth support or their own login.
- Copilot's component formats come from its CLI reference, not a published
  schema. Behaviours still to confirm in Copilot itself are tracked in
  `docs/next-steps.md`.

## Alternatives considered

- **A link step for environment-specific configuration.** Plugin MCP
  configuration cannot carry secrets, so the common case has nothing
  environment-specific in it.
- **Typed tool contracts.** Copilot cannot use them; the server is the
  authority on its tools.
- **Copy enterprise documents into every skill.** It multiplies text, and the
  guidance is not always on.
- **Emit Copilot fields by default.** Output would be non-portable without the
  author asking.
