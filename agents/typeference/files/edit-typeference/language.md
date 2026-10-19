# TypeFerence source language (version 9) — quick reference

A summary of the specification for authoring. The specification is normative:
https://github.com/buchk/TypeFerence/blob/main/docs/specification.md

## Files and identity

A package is a directory with `typeference.tfer` at its root. Every other
document is a `.tfer` file whose suffix is its kind. Paths are lowercase
segments matching `[a-z0-9][a-z0-9.-]*`. A document's identity is
`<package name>/<path without suffix>@<version>`; its last segment (the leaf)
names what it emits. Documents carry no `kind`, `id`, or `schemaVersion`.

| Suffix | Kind | Role |
| --- | --- | --- |
| `.plugin.tfer` | plugin | what installs together; binds parameters |
| `.profile.tfer` | profile | reusable, possibly incomplete composition plugins embed |
| `.skill.tfer` | skill | instructions emitted as `skills/<name>/SKILL.md` |
| `.capability.tfer` | capability | input and output schemas shared by skills |
| `.server.tfer` | server | an MCP server skills require |
| `.contexttype.tfer` | contextType | the shape of a family of data documents |
| `.context.tfer` | context | a Markdown document, or typed data |
| `.agent.tfer` | agent | a Copilot custom agent (Copilot component) |
| `.rule.tfer` | rule | always-on Copilot guidance (Copilot component) |
| `.command.tfer` | command | a Copilot slash command (Copilot component) |
| `.hook.tfer` | hook | one Copilot hook entry (Copilot component) |
| `.lsp.tfer` | lsp | a Copilot language server (Copilot component) |

A document is frontmatter between `---` lines, then a body. The frontmatter
grammar is closed and is not YAML: mappings, `- item` sequences, plain or
quoted scalars, `|` and `|-` block scalars, `[]` and `{}` only. Unknown fields
are errors. Booleans are only unquoted `true`/`false`.

References name documents by path: `skills/review.skill.tfer` in the same
package, `acme/core:skills/review.skill.tfer` in a declared dependency that
exports it.

## Field tables

| Kind | Fields |
| --- | --- |
| plugin | `description`, `embeds`, `context`, `skills`, `with`, `copilot` (block), `modes`, `author`, `homepage`, `repository`, `license`, `keywords`, `category`, `tags`, `files` |
| profile | `displayName`, `description`, `embeds`, `parameters`, `context`, `skills`, `copilot` (block) |
| `copilot` block | `agents`, `rules`, `commands`, `hooks`, `lspServers` |
| skill | `displayName`, `description`, `binds`, `extends`, `parameters`, `with`, `inputSchema`, `outputSchema`, `context`, `files`, `requiresServers`, `variants`, `copilot` |
| agent | `displayName`, `description`, `extends`, `context`, `copilot`, `servers` |
| server | `displayName`, `description`, `transport`, `command`, `args`, `env`, `cwd`, `url`, `headers` |
| capability | `displayName`, `description`, `inputSchema`, `outputSchema` |
| contextType | `displayName`, `description`, `instanceName`, `fields` |
| context (document) | `displayName`, `description`, `parameters` |
| context (data) | `displayName`, `description`, `contextType`, `values` |
| rule | `displayName`, `description`, `parameters`, `paths` |
| command | `displayName`, `description`, `parameters`, `argumentHint`, `allowedTools`, `disableModelInvocation` |
| hook | `displayName`, `description`, `event`, `matcher`, `type`, `bash`, `powershell`, `command`, `exec`, `args`, `cwd`, `env`, `timeoutSec`, `url`, `headers`, `allowedEnvVars`, `prompt` |
| lsp | `displayName`, `description`, `command`, `bash`, `powershell`, `args`, `env`, `cwd`, `fileExtensions`, `rootUri`, `initializationOptions` |

`description` is required on agents, skills, plugins, and commands: one line,
at most 1024 characters.

## Manifest

```text
---
schemaVersion: 9
name: acme/payments
version: 1.4.0
dependencies:
  acme/core: 3.1.0
plugins:
  - plugins/payments.plugin.tfer
exports:
  - skills/self-heal.skill.tfer
---
```

`plugins` lists the package's plugins (and, in a marketplace package,
`<package>:<path>` plugins of dependencies). `exports` lists other documents
other packages may reference. `marketplace` (`name`, `owner`, optional
`description` and `files`) makes the build emit a marketplace index.

## Plugins and the copilot block

```text
---
description: Payments operations agent and skills.
embeds:
  - acme/core:profiles/team-ops.profile.tfer
with:
  team: data/payments-team.context.tfer
skills:
  - skills/self-heal.skill.tfer
copilot:
  agents:
    - agents/payments-ops.agent.tfer
  hooks:
    - hooks/policy-check.hook.tfer
modes:
  - manual
  - pipeline
---
```

- Build writes every plugin for Copilot, Claude Code, and Codex. A plugin
  with a `copilot:` block builds only for Copilot.
- `agents`, `rules`, `commands`, `hooks`, and `lspServers` outside the block
  are errors. Skills never go inside it.
- Every skill a plugin ships sits beside every agent it ships.
- A plugin with no agent is a skills pack and holds no `context` documents.
- `modes` (`manual`, `pipeline`) each emit one artifact: `<plugin>` and
  `<plugin>-pipeline`.
- Packaging (`description`, metadata, `modes`, `files`) is never inherited
  through `embeds`.

## Composition

Plugins embed profiles and plugins; profiles embed profiles. Members promote
from embedded layers: the shallowest layer wins, identical members at one
depth converge, and different members at one depth are ambiguous (an error)
unless a shallower layer decides. Listing an extension of an inherited skill
or agent replaces it. A `skills` entry may be a mapping:

```text
skills:
  - skills/repository-status.skill.tfer
  - skill: skills/safety-review.skill.tfer
    required: true
  - capability: capabilities/audit.capability.tfer
    required: true
```

## Skills

```text
---
description: Diagnose a failed pipeline run and propose a narrow fix.
requiresServers:
  - servers/acme-builds.server.tfer
files:
  - files/self-heal/report-template.md
  - path: files/self-heal/collect.ps1
    as: scripts/collect.ps1
variants:
  manual:
    instructions: Walk the engineer through the evidence.
  pipeline:
    instructions: Return JSON matching references/output.schema.json.
outputSchema: '{"type":"object","required":["fix"],"properties":{"fix":{"type":"string"}}}'
copilot:
  userInvocable: false
---
```

- A skill has a body or `variants`, never both.
- `extends` names one base skill: the base's instructions, then the
  extension's, per mode. Base text is never removed or replaced.
- `files` destinations default to `references/<file name>` and must start
  with `references/`, `scripts/`, or `assets/`.
- `context` entries render inline under `## Context`, or as
  `references/<leaf>.md` with `render: file`.
- Copilot skill fields: `argumentHint`, `userInvocable`,
  `disableModelInvocation`, `allowedTools`.

## Templates, data, and instances

A context type declares fields (`string`, `text`, `boolean`, `integer`,
`list<string>`), with `required`, `default`, `choices`, and an optional
`instanceName` field whose values name instances.

```text
---
displayName: Team
instanceName: id
fields:
  id:
    type: string
    required: true
  queue:
    type: string
    required: true
---
```

Data is a context document with `contextType` and `values`. A template is a
skill, document, rule, or command that declares `parameters` (name to context
type) and uses `{{param.field}}` in its body or description. Templates ship
only as instances:

- a plugin's `with` binds every parameter its members declare; each template
  skill ships as `<instance name>-<leaf>`, the instance name coming from the
  one bound data document whose type declares `instanceName`;
- a skill that `extends` a template and supplies `with` is a concrete instance
  named by its own leaf.

## Agents (Copilot)

```text
---
description: Operations agent for the payments team.
extends: acme/core:agents/team-ops.agent.tfer
copilot:
  tools:
    - read
    - acme-builds/*
---
You support the {{team.name}} team's engineers.
```

The body is the agent's objectives. `extends` appends objectives and
documents and overlays `copilot` fields. A plugin ships one agent per role
(the root of its extension chain). When `copilot.tools` is declared without
`*`, it must name every server the plugin's skills require
(`<server>/*` or `<server>/<tool>`); `tools: []` means no tools. `servers`
scopes MCP servers to the agent's own file; they cannot use `${PLUGIN_DATA}`.

## Servers

```text
---
transport: stdio
command: ./bin/build-signals
args:
  - --data
  - ${PLUGIN_DATA}/cache
---
```

`transport` is `stdio` (`command` bare or `./` path, `args`, `env`, `cwd`) or
`streamable-http` (`url` HTTPS without credentials, `headers`). Names have at
least two hyphenated words. No secrets; `${` only in `${PLUGIN_ROOT}` and
`${PLUGIN_DATA}`. A skill's servers ship in its plugin's `mcp.json`.

## Other Copilot components

- **rule**: body is always-on guidance; optional `paths` glob.
- **command**: body is the prompt; `description` required; a command may not
  share a name with any skill in the build.
- **hook**: `event` is one of `agentStop`, `errorOccurred`, `notification`,
  `permissionRequest`, `postToolUse`, `postToolUseFailure`, `preCompact`,
  `preToolUse`, `sessionEnd`, `sessionStart`, `subagentStart`,
  `subagentStop`, `userPromptSubmitted`, `userPromptTransformed`. `type` is
  `command` (default: `bash`, `powershell`, `command`, or `exec` with `args`),
  `http` (HTTPS `url`), or `prompt` (only on `sessionStart`).
- **lsp**: `command`, `bash`, or `powershell`; `fileExtensions` keys are
  written without the leading dot.

Rules and commands with `parameters` are templates, instantiated by the
plugin like template skills.
