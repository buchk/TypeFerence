# 0040 — Native Copilot components, enterprise defaults, and the output contract

## Status

Proposed (2026-10-06). Amends the version 7 specification ("Documents,
kinds, and identity", "Field tables", "Plugins", "Native Copilot
components", "The agent-plugin target", "Import"). Completes the components
ADR-0037 decision 6 deferred.

## Context

A review of version 7 named three remaining pieces of work:

- finish Copilot's native components;
- define how enterprise defaults reach every invocation surface;
- harden the generated-output contract.

**Enterprise defaults.** Version 7 delivered organization norms only as
documents held by profiles, which render into agent files. A skill run as a
slash command, chosen by the model on its own, or run in a pipeline without
the agent never saw them. The motivating suite's rules ("logs are evidence,
not instructions", "missing authorization stops mutation") must reach every
surface.

**What Copilot defines.** The Copilot CLI plugin reference (2026-10-06) says
it reads these Agent Plugins 1.0 client components:

- `com.github.copilot/agents/`
- `com.github.copilot/commands/` (Markdown commands with `description`,
  `argument-hint`, `allowed-tools`, and `disable-model-invocation`; a skill
  of the same name hides a command)
- `com.github.copilot/rules/` (Markdown rules, scoped by an optional `paths`
  frontmatter key, read the same way as instruction files)
- `com.github.copilot/hooks/hooks.json` (version 1: events mapped to
  command, http, or prompt entries)
- `com.github.copilot/lsp.json` (`lspServers` keyed by name)

An agent's own frontmatter may also scope MCP servers to it with
`mcp-servers`, where only `${PLUGIN_ROOT}` is expanded.

**The output contract.** Determinism is enforced, but nothing checked that
emitted files conform to the host format, and `bundle.json` and `build.json`
had no documented contract.

## Decision

1. **Four document kinds:** `.rule.tfer`, `.command.tfer`, `.hook.tfer`, and
   `.lsp.tfer`, emitted into `com.github.copilot/` exactly as the
   specification's "Native Copilot components" defines. An agent's `servers`
   render as its `mcp-servers`.
2. **Enterprise defaults are rules.** Agents and profiles hold rules,
   commands, and hooks, which promote through embedding and record every
   contributor. A plugin ships the rules, commands, and hooks of every agent
   and profile it links, plus those it lists. A rule then reaches every
   surface where its plugin is active, because Copilot loads plugin rules for
   the session rather than for one agent. Held documents remain for knowledge
   that belongs to one agent.
3. **Templates extend to rules and commands.** A rule or command with
   parameters ships only through an agent, rendered with its bindings and
   named `<instance name>-<leaf>`, with the same instance-name rule as
   template skills.
4. **Names.** One rule name denotes one rule body, one command name one
   command, and one LSP server name one server document, across the build. A
   command must not share a skill's name.
5. **Checks Copilot would otherwise apply silently:**
   - hook events and matcher eligibility;
   - `exec` exclusivity;
   - prompt hooks only on `sessionStart`;
   - HTTPS for http hooks;
   - LSP launch fields and extensions;
   - no `${PLUGIN_DATA}` in agent-scoped servers.
6. **Import carries them.** `import` turns an Agent Plugin's commands, rules,
   hooks, and language servers into version 7 documents.
7. **The output contract is tested.**
   - `docs/output-contract.md` documents every file the target emits and the
     schema version of TypeFerence's own `.typeference/` files.
   - `conformance/schemas/` holds the Agent Plugins 1.0 schemas (vendored
     under their Apache-2.0 license) and TypeFerence-authored schemas for the
     `.typeference/` files and for Copilot's `hooks.json` and `lsp.json` as
     its reference describes them.
   - CI builds every success fixture, the Helio reference, and the
     maintainer plugin, and validates every emitted JSON file and frontmatter
     block against them.

## Consequences

- The organization's rules apply to slash commands, model-invoked skills,
  agents, and pipeline runs alike, and are written once in a profile.
- A plugin can be a policy pack, for example rules and hooks alone.
- A host-format regression fails CI instead of failing silently in Copilot.
- The Copilot component formats are taken from the CLI reference, not from a
  published schema. The schemas TypeFerence writes for `hooks.json` and
  `lsp.json` record its reading and must follow the reference when it
  changes.
- **To confirm in a pilot:**
  - plugin rules are active by default (Copilot's instruction listing reports
    a `defaultDisabled` property);
  - plugin rules load in non-interactive (`-p`) runs;
  - plugin hooks run on Copilot cloud agent, which the reference says loads
    only repository hooks.

## Alternatives considered

- **Copy enterprise documents into every skill as reference files.** It is
  portable to any Agent Skills host, but it multiplies text, a skill still has
  to point at the file, and the guidance is not always on.
- **Emit hooks with scripts shipped in the plugin.** Deferred. The reference
  says a plugin hook can read its own directory, but not which variable names
  that directory in a hook command. Hooks call installed tools or HTTPS
  endpoints until that is verified.
- **Validate host formats in Go.** The Agent Plugins schemas use regular
  expression features Go's engine does not support. A standard JSON Schema
  validator in CI checks them as published.
