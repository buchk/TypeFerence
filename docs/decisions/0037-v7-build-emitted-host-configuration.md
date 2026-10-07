# 0037 — Version 7: MCP servers and Copilot fields emitted by build

## Status

Proposed (2026-10-06). On acceptance: amends the version 7 specification
("Servers", "Copilot fields", "The agent-plugin target"). Replaces the tool
and deployment-linking model of ADR-0017, ADR-0022, and ADR-0029 decision 7.
Companion to ADR-0035 and ADR-0036.

## Context

Version 6 split MCP configuration across three places:

- source declared typed tool contracts;
- a separate deployment file named the providers;
- `link` wrote each plugin's `mcp.json`.

That indirection was built for multiple hosts and environments. Facts checked
against the Agent Plugins 1.0 specification and the Copilot CLI plugin
reference (2026-10-06) make it unnecessary for plugins:

- A plugin's `mcp.json` holds `stdio` servers (`command`, `args`, `env`,
  `cwd`) and `streamable-http` servers (`url`, `headers`). Values are visible
  package data and must not hold secrets.
- Conformant plugins must not depend on inherited environment variables, and
  Copilot passes only `PATH` to stdio servers. Copilot expands only
  `${PLUGIN_ROOT}` and `${PLUGIN_DATA}`, and passes remote server values
  through literally.
- Copilot handles OAuth for remote servers through discovery and dynamic
  client registration. A plugin cannot carry a client ID. Whether sign-in
  works is a property of the server.
- MCP server names resolve last-wins: a plugin's server replaces a user's own
  server of the same name. Skills and agents resolve first-found-wins.
- Nothing in the format ties a skill to the server it needs. `allowed-tools`
  pre-approves tool use; it does not declare a dependency.
- Copilot reads extra skill frontmatter (`argument-hint`, `user-invocable`,
  `disable-model-invocation`) and agent frontmatter (`tools`, `model`, and
  others) beyond the portable formats.

## Decision

1. **Server documents.** A `.server.tfer` document declares one MCP server:
   - `transport: stdio`, with `command`, `args`, `env`, and `cwd`; or
   - `transport: streamable-http`, with `url` and `headers`.

   Its name is its identity leaf. It must match `[a-z0-9]+(-[a-z0-9]+)+`, at
   least two segments, so that every server name is namespaced (for example
   `helio-tickets`) and cannot replace a user's generically named server.
   Other rules:
   - `command` is a bare executable name or a `./` plugin-relative path;
   - `url` is absolute HTTPS;
   - no value may contain `${` except `${PLUGIN_ROOT}` and `${PLUGIN_DATA}`;
   - values must not contain secrets, which the compiler cannot detect and
     reviewers must check.
2. **Skills require servers.** A skill lists servers in `requiresServers`,
   and a variant may add more. Extensions accumulate their base's.
3. **Build emits `mcp.json`.** Each plugin artifact whose shipped skills
   require servers, in the artifact's mode, gets an `mcp.json` with the
   Agent Plugins 1.0 `$schema` and one `mcpServers` entry per required
   server, sorted by name. A required server is therefore always shipped with
   the skill that requires it.
4. **One configuration per server name.** Across a build, including a whole
   marketplace, one server name denotes one server document. Two plugins
   shipping the same server ship identical configuration.
5. **Copilot fields are explicit opt-ins.** A `copilot` mapping on skills and
   agents holds Copilot-only frontmatter, emitted in a fixed order:
   - skills: `argumentHint`, `userInvocable`, `disableModelInvocation`,
     `allowedTools`;
   - agents: `model`, `tools`, `userInvocable`, `disableModelInvocation`.

   They are emitted as Copilot's kebab-case keys. `allowedTools` pre-approves
   tool use for everyone who installs the plugin, so it is never inferred
   from `requiresServers`. An agent-only skill is one an author marks
   `userInvocable: false`.
6. **Deferred:**
   - hooks, commands, rules, LSP configuration, and agent-scoped
     `mcp-servers`, until their Copilot shapes are inventoried from the
     reference;
   - per-environment server addresses, until a team needs them.

## Consequences

- A plugin is complete when built. The deployment file, `link`, and the
  `.typeference/link.json` requirements manifest disappear.
- "This skill needs that server" is checked at build time, because the
  format itself has no way to say it.
- Server naming rules protect users' own MCP configuration from accidental
  replacement.
- Credentials stay out of TypeFerence entirely. Servers that need sign-in
  rely on Copilot's OAuth support or manage their own login.

## Alternatives considered

- **Keep typed tool contracts with schemas.** Copilot cannot use them, and
  the remote server is the authority on its tools. A dependency on the
  server is what can actually be checked.
- **Infer `allowed-tools` from `requiresServers`.** It would silently grant
  permissions to everyone who installs the plugin.
- **Allow any server name.** A plugin's server named `github` would replace
  every user's own GitHub MCP configuration.
- **Emit Copilot fields by default.** That would make output non-portable
  without the author asking for it.
