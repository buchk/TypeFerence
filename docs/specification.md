# TypeFerence Draft Specification

Status: experimental reference draft, amended October 2026 to version 7. A
version 7 source package declares `schemaVersion: 7` in its project manifest.
Its resource documents carry no schema version, kind, or identity field; the
manifest and each document's path supply them. Unsupported fields and schema
versions are errors rather than extension points.

TypeFerence is an authoring and reuse layer for Agent Plugins, with GitHub
Copilot as its only output adapter (ADR-0001). Earlier source versions are not
accepted. Design rationale is in `docs/decisions/`.

## Purpose and pipeline

TypeFerence defines a closed, deterministic source language for authoring
GitHub Copilot plugins and compiling them into Agent Plugins 1.0 packages and a
Copilot marketplace index. An organization writes its skills, agents, documents,
typed data, and MCP servers once, specializes them per team, and ships them
through one marketplace. It separates three operations:

1. **restore** resolves declared source-package dependencies into a locked local
   package tree;
2. **build** resolves typed resources and emits deterministic, complete plugin
   artifacts without network access;
3. **publish** places build output where hosts read it. Publication is outside
   TypeFerence.

The output of each earlier phase is complete input to the next. Build never
implicitly restores.

`typeference import` (see "Import") is an authoring aid outside this pipeline:
it writes new source, never artifacts.

TypeFerence does not define an inference runtime, package registry server,
installer, secret manager, model credentials, MCP server implementation, or
deployment orchestrator. Copilot installs, enables, authenticates, and runs what
TypeFerence emits.

## Identity and addressing

**Identity is source; address is deployment.** Package names, versions,
document paths and the identities derived from them, dependency declarations,
the lock graph, and the content of emitted artifacts, including declared MCP
server configuration, are source. Feed and repository addresses, credentials,
marketplace repository addresses, which repositories or enterprises enable
which plugins, and runtime process details are deployment.

Deployment metadata MUST NOT occur in a source manifest and MUST NOT
participate in composition, target compilation, source package identity, or
target identity. Feed URLs and credentials identify where bytes can be
retrieved, not what those bytes mean, and MUST NOT appear in a lockfile or
source digest.

A plugin's descriptive metadata (see "Plugin metadata"), including its
`homepage` and `repository` URLs, is source: it is shown to people who browse
a marketplace, never used to retrieve anything, and part of the emitted bytes.
The same holds for files a plugin or marketplace carries (see "Carried
files"): TypeFerence does not interpret their content, so an author who writes
a deployment fact into one makes it source.

## Project manifest

A version 7 source root contains `typeference.tfer`:

```text
---
schemaVersion: 7
name: helio/payments
version: 1.4.0
marketplace:
  name: helio-agents
  owner: Helio Platform
  files:
    - files/marketplace/README.md
    - path: files/marketplace/validate.yml
      as: .github/workflows/validate.yml
dependencies:
  helio/core: 3.1.0
plugins:
  - plugins/payments.plugin.tfer
exports:
  - skills/self-heal.skill.tfer
---
```

The manifest is written in the frontmatter grammar and has no body.
`schemaVersion` is the unquoted token `7`. `name` is a lowercase package name of
two or more `/`-separated segments, each matching `[a-z0-9][a-z0-9.-]*`; the
name `typeference/builtin` and every name beneath it are reserved. `version` and
every dependency version are exact semantic versions (`MAJOR.MINOR.PATCH` with
an optional prerelease); range solving is not defined. A package cannot depend
on itself.

- `marketplace` is optional. When present, `name` is at most 64 characters of
  lowercase letters, digits, `.`, and `-`, starting and ending with a letter or
  digit, and `owner` is a required non-empty display name. Build then emits the
  marketplace index (see "The agent-plugin target"); it never invents either
  value. `files` optionally lists files the marketplace repository carries at
  its root, such as a README, a license, or CI workflows (see "Carried
  files").
- `plugins` lists the plugins the package ships: its own plugin documents
  (`.plugin.tfer` paths), and plugins of direct dependencies written
  `<package>:<path>` (see "Organization marketplaces").
- `exports` lists the package's own documents, other than plugins, that other
  packages may reference.

A manifest lists at least one plugin or export and names each at most once. A
package with no plugins is a library. Unknown fields, deployment fields, feed
addresses, and credentials are errors. Product entry points reject a source
root whose manifest is absent or does not declare `schemaVersion: 7`, naming
this specification.

## Documents, kinds, and identity

A version 7 source document is a `.tfer` file whose suffix is its kind:

| Suffix | Kind | Role |
| --- | --- | --- |
| `.agent.tfer` | agent | a concrete composition: identity, objectives, documents, skills, parameter bindings |
| `.profile.tfer` | profile | a reusable, possibly parameterized composition |
| `.capability.tfer` | capability | input and output schemas shared by unrelated skills |
| `.skill.tfer` | skill | instructions emitted as `SKILL.md`, possibly a template |
| `.server.tfer` | server | an MCP server that skills require |
| `.contexttype.tfer` | contextType | the shape of a family of data documents |
| `.context.tfer` | context | a document (Markdown) or data (typed values) |
| `.plugin.tfer` | plugin | an installable package: the agents, profiles, skills, and native components that ship together |
| `.rule.tfer` | rule | always-on Copilot guidance, emitted as a rule |
| `.command.tfer` | command | a Copilot slash command |
| `.hook.tfer` | hook | one Copilot hook entry and the event it handles |
| `.lsp.tfer` | lsp | a Copilot language server |

A document path is relative to the package root, uses `/`, and is clean: not
absolute, with no empty, `.`, or `..` segment. Every segment of its stem, the
path without the kind suffix, matches `[a-z0-9][a-z0-9.-]*`.

A document's identity is `<package name>/<stem>@<package version>`:
`skills/core/review.skill.tfer` in package `acme/team` version `1.2.0` is
`acme/team/skills/core/review@1.2.0`. The identity's **leaf** is its last stem
segment (`review`); leaves name emitted artifacts. Two documents whose paths
derive the same identity, such as `skills/x.skill.tfer` and
`skills/x.agent.tfer`, are an error.

Documents carry no `schemaVersion`, `kind`, or `id` field; any of them is an
unknown field.

## References

A reference names a document by path:

- `skills/review.skill.tfer` names a document in the same package;
- `helio/core:skills/review.skill.tfer` names a document in a direct
  dependency. The package MUST be declared in the manifest's `dependencies`;
  the reference denotes the identity derived from that package, its declared
  version, and the path; and that package MUST export the document. Qualifying
  a reference with the referencing package's own name is an error.

Each reference field accepts the kinds listed for it below, checked by suffix;
a reference to any other kind is an error. Plugins are listed only by a
manifest and are never referenced from a document. A reference list names each
document at most once.

Resolution is exact. It never searches, never selects a similarly named
document, and never lets a root document shadow a dependency's: an identity
present in both is an error. Moving or renaming a file changes its identity;
every reference to the old path is a build error until it is updated.

## Source document format

A document is frontmatter between two fence lines, then a body:

```text
---
description: Summarize where a repository stands. Use when asked for repository status.
---
Report the current branch, working-tree state, and failing checks.
```

The file MUST start with an opening `---` line, and the first later line that is
exactly `---` closes the frontmatter. The text between the fences is one mapping
in the frontmatter grammar (possibly empty); the text after the closing fence is
the body. Input text is UTF-8; a byte order mark is stripped and CRLF is
normalized to LF before the document is split. The body is otherwise preserved.
Trimming is used only to decide whether a body is empty.

A body belongs to its document's kind:

- a unimodal skill's body is its instructions. A multimodal skill keeps each
  rendering under `variants` and MUST NOT have a body;
- a document context's body is its Markdown text and MUST NOT be empty;
- an agent's body is its objectives (see "Agents and objectives").

Every other kind, including data contexts, and the manifest, rejects a
non-whitespace body.

### Frontmatter grammar

The frontmatter grammar is TypeFerence's own closed indentation grammar. Its
syntax is fixed by this specification; it is not YAML and has no YAML resolver
semantics. It supports exactly:

- mappings of `key: value` lines. A nested block is indented more deeply than
  its key, and every entry of one block uses the same indentation. Indentation
  is spaces; a tab in structural indentation is an error. A key matches
  `[A-Za-z0-9_][A-Za-z0-9_./-]*`, is followed by `:` and then a space or the end
  of the line, and appears at most once in its mapping. Quoted keys do not
  exist;
- sequences of `- item` lines, at their key's indentation or deeper. An item
  written `- key: value` opens a mapping whose further keys align with that
  first key;
- plain (unquoted) scalars, which run to the end of the line or to a comment;
- single-quoted scalars, in which `''` is a literal quote, and double-quoted
  scalars, which use JSON string escapes (`\"`, `\\`, `\/`, `\b`, `\f`, `\n`,
  `\r`, `\t`, `\uXXXX`; a surrogate pair combines into one character and an
  unpaired surrogate is an error). No text may follow a closing quote. A quote
  opens a quoted scalar only at the start of a value; elsewhere it is ordinary
  text, as in `don't`;
- literal block scalars for multiline text. The header is `|` (the value ends
  with exactly one line feed) or `|-` (no final line feed), optionally with an
  indentation indicator `N` from 1 to 9 written before the `-` (`|2`, `|2-`)
  that fixes the content indentation at the key's indentation plus `N`;
  otherwise the first content line fixes it. Content is every following line
  indented more deeply than the key; blank lines within it are preserved,
  trailing blank lines are not content, and `#` is ordinary text. A block
  scalar cannot be a sequence item. Folded scalars (`>`) and keep chomping
  (`|+`) do not exist;
- null, written as an empty value, `null`, or `~`;
- the empty-collection tokens `[]` and `{}`. Every other flow collection is an
  error;
- comments: outside quoted and block scalars, a `#` that begins a line's
  content or follows a space or tab starts a comment that runs to the end of
  the line. Comments are inert and never reach artifacts.

Anchors, aliases, tags, directives, and multiple documents do not exist.

### Scalar typing

Scalar typing is schema-directed. The grammar never assigns a type to a scalar;
the field that receives it declares one:

- a string field, including every reference field, takes a plain, quoted, or
  block scalar's text verbatim. A plain `no`, `true`, or `1.0` in a string field
  is that string;
- a boolean field accepts only the unquoted tokens `true` and `false`;
- an integer field accepts an unquoted JSON integer token
  (`-?(0|[1-9][0-9]*)`) within the signed 64-bit range, preserved as written;
- a quoted or block scalar is always a string. It never satisfies a boolean
  or integer field;
- a list field accepts `[]` and a mapping field accepts `{}` as empty; null
  is empty for either.

No implicit resolution anywhere chooses a type from a value's spelling.
Floating-point numbers do not exist.


### Provenance rule

Every byte an artifact contains derives from a file in the package closure or
from a rule of this specification, and provenance records the source of each
contribution. Prose is ordinary Markdown wherever a body or instruction field
accepts it; no field carries hidden behavior outside the field tables.

`displayName` and `description` are single-line strings with no control
characters or line separators. `description` is routing metadata: an emitter
places it only in `SKILL.md` and custom agent frontmatter, `plugin.json`,
marketplace entries, and bundle metadata, never in an instruction body.
`displayName` titles an agent's instructions and labels a document section.

### Field tables

| Kind | Fields |
| --- | --- |
| agent | `displayName`, `description`, `embeds` (profiles or agents), `context` (documents), `skills` (bindings), `with` (parameter name to data), `copilot`, `rules` (rules), `commands` (commands), `hooks` (hooks), `servers` (servers scoped to the agent) |
| profile | `displayName`, `description`, `embeds` (profiles), `parameters` (parameter name to context type), `context` (documents), `skills` (bindings), `rules` (rules), `commands` (commands), `hooks` (hooks) |
| capability | `displayName`, `description`, `inputSchema`, `outputSchema` |
| skill | `displayName`, `description`, `binds` (capability), `extends` (skill), `parameters`, `with`, `inputSchema`, `outputSchema`, `context` (document entries), `files`, `requiresServers` (servers), `variants`, `copilot` |
| server | `displayName`, `description`, `transport`, `command`, `args`, `env`, `cwd`, `url`, `headers` |
| contextType | `displayName`, `description`, `instanceName`, `fields` |
| context (document) | `displayName`, `description`, `parameters` |
| context (data) | `displayName`, `description`, `contextType` (context type), `values` |
| plugin | `description`, `agents` (agents), `profiles` (profiles), `skills` (skills), `modes`, `rules` (rules), `commands` (commands), `hooks` (hooks), `lspServers` (LSP servers), `author`, `homepage`, `repository`, `license`, `keywords`, `files` |
| rule | `displayName`, `description`, `parameters`, `paths` |
| command | `displayName`, `description`, `parameters`, `argumentHint`, `allowedTools`, `disableModelInvocation` |
| hook | `displayName`, `description`, `event`, `matcher`, `type`, `bash`, `powershell`, `command`, `exec`, `args`, `cwd`, `env`, `timeoutSec`, `url`, `headers`, `allowedEnvVars`, `prompt` |
| lsp | `displayName`, `description`, `command`, `bash`, `powershell`, `args`, `env`, `cwd`, `fileExtensions`, `rootUri`, `initializationOptions` |

`description` is required on agents, skills, plugins, and commands, is at most 1024
characters after field references are resolved, and must remain a single line
without control characters after they are resolved. Schemas are JSON Schema
documents written as string values and canonicalized as JSON.

## Plugins

A plugin is the solution file for one installable package. It links; it does
not compose:

```text
---
description: Payments team agent and skills.
agents:
  - agents/payments-ops.agent.tfer
skills:
  - skills/changelog.skill.tfer
modes:
  - manual
  - pipeline
---
```

A plugin links at least one agent, profile, skill, rule, command, hook, or LSP
server. A plugin that links no agents is a skills pack. `modes` lists `manual`, `pipeline`, or both, each at
most once, and defaults to `manual`.

A plugin ships:

- each linked agent, and every skill and skill instance that agent resolves to
  after composition (see "Instances");
- for each linked profile, the profile's resolved skills. The profile MUST be
  complete, binding every capability it requires, MUST declare no
  `parameters`, and MUST hold no documents, directly or through embedding:
  without an agent there is nowhere to deliver them;
- each linked skill, after flattening its extension chain;
- every server that a shipped skill requires in the artifact's mode (see
  "Servers");
- the rules, commands, and hooks of its linked agents and profiles, and the
  rules, commands, hooks, and LSP servers it lists itself (see "Native Copilot
  components").

A skill reached more than once ships once. A skill shipped through `skills` or
`profiles` MUST have no unbound parameters.

A plugin's **owning package** is the package that contains its document. Its
artifacts carry the owning package's version and provenance, whichever build
ships them (see "The agent-plugin target").

A plugin's name is its identity leaf. It MUST satisfy the Agent Plugins name
grammar: at most 64 characters of lowercase letters, digits, `.`, and `-`,
starting and ending with a letter or digit, with no `--` or `..`. When the
plugin lists `pipeline`, `<name>-pipeline` MUST satisfy it as well. For each
plugin and each of its modes:

- each linked agent's name, its identity leaf, MUST satisfy the Agent Skills
  name grammar `[a-z0-9]+(-[a-z0-9]+)*` in at most 64 characters, so that it is
  a clean custom agent name;
- each shipped skill's emitted name MUST satisfy the same grammar, and no two
  shipped skills may share an emitted name;
- each shipped multimodal skill MUST define a variant for the mode.

### Plugin metadata

A plugin may declare the descriptive members of the Agent Plugins 1.0
manifest, which hosts show when people browse or search a marketplace:

```text
---
description: Payments team agent and skills.
author:
  name: Payments Platform
  email: payments-platform@example.com
  url: https://example.com/teams/payments
homepage: https://example.com/docs/payments-agents
repository: https://example.com/git/payments-agents
license: MIT
keywords:
  - payments
  - incidents
agents:
  - agents/payments-ops.agent.tfer
---
```

- `author` is a mapping of `name` (required), `email`, and `url`;
- `homepage`, `repository`, and `license` are strings. An SPDX license
  identifier is recommended but not checked;
- `keywords` lists at least one keyword, each at most once, in authored order.

Every value is a non-empty single line without control characters or
surrounding whitespace. TypeFerence never fetches or checks a URL. Build emits
the metadata unchanged into every artifact of the plugin, in `plugin.json` and
in the plugin's marketplace entry (see "The agent-plugin target"); it never
invents a value, and a member the plugin does not declare is absent. The
manifest member `extensions` has no source form.

### Carried files

A plugin's `files` lists package files that ship at the root of each of its
artifacts, such as a README or templates people copy into their repositories.
A marketplace's `files` (see "Project manifest") lists package files that ship
at the target root, such as the marketplace repository's README, license, and
CI workflows. Both use the entry grammar of skill files:

```text
files:
  - files/payments/README.md
  - path: files/payments/settings.template.json
    as: templates/settings.json
```

`path` follows the rule for skill files (see "Skill files"). `as` is the
destination, relative to the artifact directory for a plugin and to the target
root for a marketplace, and defaults to the file name of `path`. A destination
MUST be a clean path and MUST NOT claim a path build emits itself. Compared
after lowercasing:

- a plugin file's destination MUST NOT be `plugin.json` or `mcp.json`, and its
  first segment MUST NOT be `skills`, `com.github.copilot`, `.typeference`, or
  `.git`;
- a marketplace file's first segment MUST NOT be `.typeference`, `.git`, or the
  name of an artifact directory the build emits, and the destination MUST NOT
  lie beneath `.github/plugin` or `.github/copilot`. The first is where the
  marketplace index lives; the second holds repository settings, which build
  never emits.

Destinations within one plugin's files, and within the marketplace's files,
follow the collision and directory-spelling rules of skill files (ADR-0003).
Carried files are source members, read and normalized like skill files; field
references are not resolved in them. A plugin's files ship identically in each
of its artifacts.

## Agents and objectives

An agent is identity (`description`, `displayName`), objectives (its body), and
composition (`embeds`, `skills`, `context`, `with`). Objectives render
immediately after the agent's title. Knowledge a whole team needs is a document
the agent or its profiles hold; facts that vary per team are data the agent
binds with `with`. An agent's objectives and description MAY reference the
parameters it binds (see "Parameters and field references"). An agent or
profile without a `displayName` takes its identity leaf.

Agents are concrete: they declare no parameters and MUST fulfill every
promoted requirement and bind every promoted parameter.

## Composition

Agents MAY embed profiles or agents. Profiles MAY embed profiles but MUST NOT
embed agents. An embedding graph MUST NOT contain a cycle.

Resolution proceeds from embedded resources toward the embedding resource:

1. Display name and description belong to their declaring resource.
2. Held document references append in embedding order and deduplicate in
   first-seen order. Provenance records every contributing profile or agent for
   every held document, including each contributor of a document several
   layers hold.
3. Capability bindings promote by capability identity. Requirements
   accumulate: a capability is required when any layer at any depth requires
   it, and an abstract requirement (a binding with no skill) adds that
   obligation without selecting, replacing, or erasing an implementation.
   Among implementations, the shallowest wins. At the same depth, bindings with
   the same resolved implementation converge as one member and provenance
   retains every contributor; different implementations are ambiguous unless
   bound by a shallower layer.
4. Parameter declarations promote by name. Declarations of one name MUST name
   one context type; otherwise the composition is an error. An agent's
   parameter bindings (`with`) promote from the agents it embeds by the same
   rule as capability bindings: the shallowest binding of a name wins,
   identical bindings at one depth converge, and different bindings at one
   depth are ambiguous unless a shallower layer binds the name.
5. Objectives inherit: an agent's resolved objectives are its embedded agents'
   objectives in embedding order, then its own body, each source at most once
   in first-seen order.
6. Every contribution records source-resource provenance.

Profiles may retain abstract required capability bindings. `required` is the
demand side of composition: a binding with no skill declares an obligation
without supplying an implementation.

## Capabilities, skills, and bindings

A capability document defines canonical JSON `inputSchema` and `outputSchema`
for skills that share a contract but not text. A skill that `binds` a
capability takes the capability's schemas and MUST NOT declare its own.

A skill that neither binds nor extends defines an **implied capability**,
identified by the skill's own identity, with the skill's schemas. An extension
shares its base's capability (see "Skill extension"). A capability reference in
a binding's `capability` names a capability document or a skill; naming a skill
denotes that skill's capability.

A `skills` entry in an agent or profile is a skill path or a mapping:

```text
skills:
  - skills/repository-status.skill.tfer
  - skill: skills/safety-review.skill.tfer
    required: true
  - capability: capabilities/audit.capability.tfer
    required: true
```

A mapping takes `skill`, `capability`, and `required`. An entry that names a
skill binds that skill's capability; if it also names a capability, the skill
MUST implement it. An entry without a skill MUST name a capability and set
`required: true`. `required` demands that concrete agents contain a binding.

Binding a skill whose capability an embedded layer already binds rebinds that
capability under rule 3 of "Composition".

## Skill extension

A skill MAY name exactly one base skill in `extends`. Chains are allowed; a
cycle is an error. A skill declares `binds` or `extends`, never both.

- **Same contract.** An extension takes its base's capability and schemas and
  MUST NOT declare its own.
- **Additive instructions.** An extension's instructions are its base's
  flattened instructions, verbatim, followed by its own, joined by exactly one
  blank line: the base text without its trailing line feeds, two line feeds,
  then the extension's text without its leading line feeds. Per mode:
  - base and extension unimodal: one concatenation;
  - base unimodal, extension multimodal: each extension variant is the base
    text followed by that variant's text, and the extension's modes are the
    result's modes;
  - base multimodal, extension unimodal: each base variant is followed by the
    extension's text;
  - both multimodal: the extension MAY supply variants only for modes the base
    defines, and a new mode is an error. Each base mode renders the base text
    followed by the extension's text for that mode, or the base text alone.

  An extension with no instructions and no variants keeps its base's text
  unchanged. Replacing or removing base text is not expressible; that is a new
  skill.
- **Requirements and resources accumulate.** `requiresServers`, held
  `context`, `files`, and `parameters` are the base's followed by the
  extension's, deduplicated in first-seen order; a mode's variant requirements
  accumulate the same way. A parameter name declared on both MUST name one
  context type. Two `files` entries with one destination are an error.
- **Own identity and routing.** An extension has its own identity, emitted
  name, and required `description`.
- **Instantiation.** An extension that supplies `with` instantiates its
  flattened chain (see "Instances").

## Invocation modes

A skill declares instructions (its body) or `variants`, never both. Mode names
are `manual` and `pipeline`. Base requirements apply to every mode; variant
requirements are additive:

```text
---
description: Diagnose a failed pipeline run and propose a narrow fix.
requiresServers:
  - servers/helio-tickets.server.tfer
variants:
  manual:
    instructions: Walk the engineer through the evidence.
  pipeline:
    instructions: Return JSON matching references/output.schema.json.
    requiresServers:
      - servers/helio-builds.server.tfer
---
```

A variant takes `instructions` (required) and `requiresServers`. For mode `m`,
effective requirements are `base ∪ variant[m]`. An agent-plugin artifact renders
exactly one mode. Variants MUST NOT change the bound capability or its schemas.

## Documents and data

A context document is exactly one of:

- a **document**: Markdown in its body. It MAY declare `parameters` and
  reference them in its body. It has no type;
- **data**: a `contextType` and `values`, with no body. Data is never rendered
  on its own; its values reach artifacts only through field references.

```text
---
displayName: Reply style
---
Replies are short and lead with the answer.
```

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

Agents, profiles, and skills hold documents with `context`; holding a data
document is an error. Data is bound to parameters with `with`.

## Context types

A context type is the shape of a family of data documents and the contract
that everyone instantiating a template against it fulfills:

```text
---
displayName: Team
description: What a team supplies to instantiate the team operations kit.
instanceName: id
fields:
  id:
    type: string
    required: true
    displayName: Team identifier
    description: Lowercase, hyphenated; prefixes the team's skill names.
  name:
    type: string
    required: true
    displayName: Team name
  queue:
    type: string
    required: true
    displayName: Ticket queue
  tier:
    type: string
    default: standard
    choices:
      - standard
      - critical
  repositories:
    type: list<string>
---
```

`fields` maps field names, matching `[A-Za-z][A-Za-z0-9_]*`, to declarations in
authored order. A declaration takes:

- `type`, one of `string`, `text` (multiline string), `boolean`, `integer`, or
  `list<string>`;
- `required`, a boolean defaulting to `false`;
- `default`, a value of the field's type. A required field MUST NOT declare a
  default;
- `displayName` and `description`, form metadata for the field;
- `choices`, for `string` fields only: a non-empty list of distinct strings.
  The default and every value MUST be one of them.

`instanceName`, when present, names a `string` field that is required or has a
default. Every value of that field, including the default, MUST satisfy the
Agent Skills name grammar `[a-z0-9]+(-[a-z0-9]+)*`.

A data document's values follow the scalar typing rules against their declared
field types. Missing required fields, unknown fields, wrong shapes, and values
outside `choices` are errors. Defaults are materialized before hashing and
emission.

A context type's ordered field declarations, with their names, types,
requiredness, defaults, metadata, and choices, are sufficient to generate an
input form whose result is a data document. Generating forms is outside this
specification.

## Parameters and field references

A document, skill, or profile MAY declare `parameters`, a mapping from
parameter names, matching `[a-z][A-Za-z0-9]*`, to context types. A skill or
document that declares parameters, directly or through its extension chain, is
a **template**. A template is never emitted directly; it is emitted only as an
instance (see "Instances"). A profile's parameters declare its contract: every
member skill and held document that declares a parameter of the same name MUST
declare the same context type.

In a template's body, variant instructions, and `description`, and in an
agent's objectives and description, `{{name.field}}` is a **field reference**.
`name` is a parameter of the template, or a parameter the agent binds, and
`field` is a field of its context type. The reference is replaced by the bound
value:

- `string` and `text` values are inserted verbatim, including any line feeds;
- `integer` values are inserted as written;
- `boolean` values are inserted as `true` or `false`.

An undeclared parameter, an unknown field, a `list<string>` field, and an
optional field with neither a value nor a default are errors. References are
replaced exactly once; inserted values are never scanned for further
references. `\{{` writes a literal `{{`. In a document or skill that declares no
parameters, and in an agent that binds none, `{{` is ordinary text.

A field reference matches exactly `{{` followed by a parameter name, `.`, a
field name, and `}}`, with no whitespace. Other text containing `{{` is
ordinary text even in a template.

## Instances

Templates are instantiated in two ways.

**Skill instances.** A skill that `extends` a template and supplies `with`, a
mapping from each parameter of its flattened chain to a data document whose
`contextType` is the parameter's type, is a concrete skill. It MUST bind every
parameter, MUST NOT bind a name that is not a parameter, and MUST NOT declare
`parameters` of its own. Its emitted name is its own identity leaf.

**Agent instances.** An agent's `with` maps parameter names to data documents.
Its bindings are its own `with` together with those promoted from the agents
it embeds (rule 4 of "Composition"), so embedding an agent and binding a name
re-points the embedded agent at other data, while embedding it without binding
keeps the embedded agent's data. Every objective, including an embedded
agent's, renders with the embedding agent's bindings. After composition, the
agent's parameterized members are the templates among its resolved skills,
those skills' held documents, and its held documents. The agent's bindings:

- MUST bind every parameter those members and its embedded profiles declare,
  with data of the declared context type;
- MUST NOT, in its own `with`, bind a name that is neither declared by
  something it composes nor referenced by its objectives or description.

Each template skill the agent resolves to is emitted as an instance named
`<instance name>-<skill leaf>`. The instance name is the value of the
`instanceName` field of the one bound data document whose context type declares
`instanceName`. When the agent resolves to at least one template skill, exactly
one bound data document MUST supply an instance name. A skill that is not a
template keeps its own name. A rebinding that replaces a template skill also
removes that skill's parameters from what the agent must bind.

One agent binds one value per parameter name. Every emitted instance name is
subject to the build-wide name rules (see "The agent-plugin target").

A contract change is visible everywhere: adding a required field to a context
type fails every data document of that type that lacks it, in every package a
build includes.

## Rendering held documents

A skill's `context` entry is a document path or a mapping with `context` (a
document path) and `render` (`inline`, the default, or `file`).

- **Inline** documents render after the skill's instructions in a `## Context`
  section, each under `### ` and its `displayName` or identity leaf, followed by
  its body with field references resolved.
- **File** documents render as `references/<leaf>.md`, holding the body with
  field references resolved and nothing else. The skill's instructions are
  responsible for pointing at the file.

An agent's held documents always render inline in its agent file under
`## Context`, in resolved order.

## Skill files

A skill's `files` lists package files that ship in its skill directory. An entry
is a path or a mapping with `path` and `as`:

```text
files:
  - files/self-heal/report-template.md
  - path: files/self-heal/collect.ps1
    as: scripts/collect.ps1
```

`path` is a clean, package-relative path to an existing file that is not a
`.tfer` document and not beneath `dist`, `bin`, or `obj`. `as` is the
destination relative to the skill directory and defaults to
`references/<file name>`. A destination MUST be a clean path whose first segment
is `references`, `scripts`, or `assets`, and MUST NOT collide with another
file, a rendered document, or an emitted schema in the same skill directory.
Two destinations collide when they are equal after lowercasing, because they
name one file on case-insensitive filesystems (ADR-0003).
Every shared directory prefix MUST also use identical spelling: destinations
`references/A/one.txt` and `references/a/two.txt` are an error even though
their file names differ. A path MUST NOT name both a file and a directory,
including after lowercasing. These checks apply to the complete skill directory,
including inherited files, rendered documents, and schemas, before output is
written. Accepted destination spelling is preserved (ADR-0003).

Skill files are source members. A file whose bytes are valid UTF-8 is
normalized like source text (BOM removed, CRLF to LF) and emitted normalized;
any other file is emitted byte for byte. Field references are not resolved in
skill files.

A skill that has an `inputSchema` or `outputSchema`, its own, its capability's,
or its base's, emits it as `references/input.schema.json` or
`references/output.schema.json`, canonical JSON with two-space indentation and a
final line feed, in every artifact that ships the skill. The schema an
automated runner validates against and the schema the instructions name are
thereby one source.

## Servers

A server document declares one MCP server:

```text
---
description: Helio's ticket system.
transport: streamable-http
url: https://tickets.helio.example/mcp
---
```

```text
---
transport: stdio
command: ./bin/build-signals
args:
  - --data
  - ${PLUGIN_DATA}/cache
---
```

`transport` is `stdio` or `streamable-http`.

- A `stdio` server takes `command` (required), `args` (a list of strings),
  `env` (a mapping of strings), and `cwd`. `command` is a bare executable name
  matching `[A-Za-z0-9_][A-Za-z0-9._+-]*` or a `./` path without `..`. `cwd`, when
  present, is a `./` path, `${PLUGIN_ROOT}` or `${PLUGIN_DATA}`, or one of those
  followed by `/` and a clean path.
- A `streamable-http` server takes `url` (required, an absolute HTTPS URL
  without credentials) and `headers` (a mapping of strings).

Fields of the other transport are errors. No value may contain `${` except as
part of `${PLUGIN_ROOT}` or `${PLUGIN_DATA}`, which the host expands. Values are
visible package data: they MUST NOT contain credentials or other secrets.

A server's name is its identity leaf and MUST match `[a-z0-9]+(-[a-z0-9]+)+`:
at least two hyphen-separated segments, so that every emitted server name is
namespaced. Copilot gives a plugin's server precedence over a user's server of
the same name; namespacing keeps plugins from replacing users' generically
named servers.

Skills require servers with `requiresServers`; variants may add requirements.
Across a build, one server name denotes one server document.

## Copilot fields

A skill or agent MAY carry a `copilot` mapping of Copilot-only frontmatter.
These fields are explicit opt-ins; the portable Agent Plugins core never
requires them.

| Kind | Field | Type | Emitted key |
| --- | --- | --- | --- |
| skill | `argumentHint` | string | `argument-hint` |
| skill | `userInvocable` | boolean | `user-invocable` |
| skill | `disableModelInvocation` | boolean | `disable-model-invocation` |
| skill | `allowedTools` | list of strings | `allowed-tools` |
| agent | `model` | string | `model` |
| agent | `tools` | list of strings | `tools` |
| agent | `userInvocable` | boolean | `user-invocable` |
| agent | `disableModelInvocation` | boolean | `disable-model-invocation` |

`allowedTools` pre-approves tool use for everyone who installs the plugin and is
never inferred. It lists at least one tool or is omitted.

An agent's `tools` is its tool allowlist. Omitting it gives the agent Copilot's
default tools; `tools: []` gives it none and is emitted as `tools: []`; a key
without a value is an error, because it could mean either. MCP server tools
are named `<server>/<tool>` or `<server>/*`, and `*` names every tool. When an
agent declares `tools` without `*`, then in every artifact that ships the
agent, each server that a skill the agent binds requires in that artifact's
mode, and each server in the agent's own `servers`, MUST be named by an entry
`<server>/*` or `<server>/<tool>`. Copilot enables a server's tools for an
agent only when the agent's `tools` names them, and build never adds an entry,
so a missing one fails compilation. Entries naming servers an artifact does not
ship are emitted unchanged; Copilot ignores tool names it does not
recognize. A skill meant to be used only by its agent sets
`userInvocable: false`. Extensions inherit their base's `copilot` fields; an
extension's own fields replace the base's field by field.

## Native Copilot components

Agent Plugins 1.0 defines skills and MCP servers as its portable components.
Copilot reads further components from the plugin's `com.github.copilot/`
directory; TypeFerence declares each as a document kind (ADR-0007).

**Rules** (`.rule.tfer`) are always-on guidance. A rule's body is Markdown; its
optional `paths` is one glob that scopes it to matching files, and its
optional `description` labels it. A rule is emitted as
`com.github.copilot/rules/<name>.md`, with frontmatter (`paths`, then
`description`) only when one of them is declared.

**Commands** (`.command.tfer`) are slash commands. A command's body is the
prompt; `description` is required, and `argumentHint`, `allowedTools`, and
`disableModelInvocation` are emitted as `argument-hint`, `allowed-tools`, and
`disable-model-invocation`. A command is emitted as
`com.github.copilot/commands/<name>.md`. A command MUST NOT share an emitted
name with any skill in the build, because Copilot lets a skill hide a command
of the same name.

**Hooks** (`.hook.tfer`) are single hook entries. `event` names a Copilot hook
event; `matcher` is allowed only on `notification`, `permissionRequest`,
`postToolUse`, `preCompact`, `preToolUse`, and `subagentStart`. `type` is
`command` (the default), `http`, or `prompt`:

- a `command` hook takes `bash`, `powershell`, `command`, or `exec` (an
  executable run without a shell, which takes `args` and excludes the
  others), with `cwd` and `env`;
- an `http` hook takes an absolute HTTPS `url`, `headers`, and
  `allowedEnvVars`;
- a `prompt` hook takes `prompt` and is allowed only on `sessionStart`.

`timeoutSec` is a positive whole number. An artifact's hooks are emitted
together as `com.github.copilot/hooks/hooks.json`: `version` `1` and `hooks`,
mapping each event, in canonical order, to its entries in hook identity order.

**LSP servers** (`.lsp.tfer`) take `command`, `bash`, or `powershell` (at least
one), `args`, `env`, `cwd`, `fileExtensions`, `rootUri`, and
`initializationOptions` (a JSON document written as a string).
`fileExtensions` maps extensions, written without their leading dot because a
frontmatter key cannot start with `.`, to language identifiers; the emitter
adds the dot. An artifact's servers are emitted together as
`com.github.copilot/lsp.json`, `lspServers` keyed by server name.

**Agent-scoped servers.** An agent's `servers` lists server documents that
render in the agent's own frontmatter as `mcp-servers`, scoping them to the
agent: `type` (`stdio` or `http`), the transport's fields, and `tools` (`*`).
Copilot expands only `${PLUGIN_ROOT}` there, so a server an agent scopes to
itself MUST NOT use `${PLUGIN_DATA}`. Agent-scoped servers do not enter the
plugin's `mcp.json`.

**Enterprise defaults.** Agents and profiles hold rules, commands, and hooks,
and they promote through embedding like held documents, recording every
contributor. A plugin ships the rules, commands, and hooks of every agent and
profile it links. A rule therefore reaches every surface where the plugin is
active, whether an agent, a slash command, a model-invoked skill, or a
pipeline run, rather than only the agent whose file holds a document. A rule
or command with parameters is a template: it ships only through an agent,
rendered with the agent's bindings and named `<instance name>-<leaf>`, and an
agent that instantiates one must supply an instance name exactly as for
template skills. Across a build, one rule name denotes one rule body, one
command name one command, and one LSP server name one server document.

## Packages, restore, and lockfiles

`typeference pack` emits one canonical source package (`.tferpkg`) from a
version 7 source root. It contains:

- package name and exact version;
- exact dependency declarations;
- the sorted identities of the manifest's exports;
- the sorted canonical paths and normalized UTF-8 contents of the package's
  source members (see "Source membership and digests");
- a `sha256:` package digest.

The package envelope is canonical JSON. Paths use `/`, may not be absolute,
contain `..`, or collide after normalization. Files are LF-normalized and
BOM-free. No archive timestamp, host path, feed address, credential, deployment
value, generated target, or VCS data is permitted. Pack requires a lockfile
that matches the declared dependencies when there are any.

`typeference restore` reads the manifest, resolves the exact dependency graph from
configured routes, verifies every package digest, rejects cycles/conflicts, writes
`typeference.lock`, and materializes the complete graph beneath
`obj/typeference/packages`. If a lockfile already exists, `restore` honors it and
MUST reject any manifest/lock/feed disagreement without rewriting it;
`restore --locked` additionally requires the lockfile to exist. `typeference
update` is the explicit operation that re-resolves and rewrites the graph after
the author changes exact manifest dependencies.

The canonical lockfile records root manifest identity, package name/version,
digest, exports, and dependency edges in deterministic order. It MUST NOT contain
feed URLs, credentials, cache paths, retrieval timestamps, or mutable tags.

Feed configuration is external. Implementations SHOULD support scoped routes and
MAY implement filesystem, generic HTTP/JFrog Generic, Azure Artifacts, and Git
transports. Routes select one feed for a namespace; implementations MUST NOT
search every configured feed. Credentials are environment/helper references and
never source.

A Git route names an HTTPS repository URL, an optional package root path within
the repository, and a tag prefix. Restore resolves a package `<name>@<version>`
by fetching the tag `<prefix><version>`, reading the version 7 source root at
the package root path, verifying that its manifest declares that name and
version, and packing it as `typeference pack` would. The resulting package
digest is what the lockfile records and what later restores verify, exactly as
for any other route; commit identifiers, URLs, and tags never enter the
lockfile. Git credentials come from Git's own credential configuration.

Build reads only the source root, lockfile, and a supplied materialized package
directory. It verifies locked digests and performs no network access. Missing,
undeclared, conflicting, or corrupt dependencies are errors. A package-qualified
reference MUST name a document its package exports. A dependency's plugins ship
in a dependent's build only when the dependent's manifest lists them (see
"Organization marketplaces").


## Source membership and digests

The source digest hashes an explicit resource set, never an arbitrary recursive
directory after output has been written. A version 7 package's source members
are:

- the project manifest;
- the lockfile, when present;
- every document in the closure of references from the manifest's own plugins
  and its exports, every file a member skill or member plugin lists in
  `files` (see "Skill files" and "Carried files"), and every file the
  manifest's `marketplace` lists in `files`. A dependency's plugin that the
  manifest lists is a member of that dependency's package, not of this one.

Files outside the closure are not members: they are neither parsed nor hashed.
The set excludes `.git`, `dist`, `bin`, `obj`, restored packages, deployment
files, feed configuration, signature maps, and generated artifacts. Build MUST
reject an output directory nested inside the source root unless the output path
is in a normatively excluded generated directory (`dist`, `bin`, or `obj`). The
source root itself is never a valid output directory.

`typeference-resource-set-v1` sorts normalized source-relative paths by UTF-8 byte
order and hashes, for each file, `path`, NUL, content, NUL using SHA-256, where
content is normalized text for `.tfer` documents and for skill and carried
files that are valid UTF-8, and the exact bytes of any other such file. Target provenance records the root source digest, the ordered locked
dependency digests, and target adapter identity. Release metadata belongs to the
distributed compiler binary, not the reproducible source-derived artifact.

`typeference-directory-v1` is the target-directory digest: recursively sort
forward-slash paths, then hash `path`, NUL, content, NUL, where content is
normalized text (byte order mark removed, CRLF to LF) for a file whose bytes are
valid UTF-8 and the exact bytes of any other file (ADR-0003). It applies only to
already-defined artifact directories, not source identity.


## The agent-plugin target

Build emits the `agent-plugin` target beneath `<out>/agent-plugin/`. It is
complete: there is no later linking step. A request for any other target fails
with a diagnostic.

```text
agent-plugin/
  .github/plugin/marketplace.json
  .typeference/build.json
  .typeference/compatibility.json
  <marketplace file>...
  <artifact>/
    plugin.json
    mcp.json
    com.github.copilot/agents/<agent>.agent.md
    com.github.copilot/rules/<rule>.md
    com.github.copilot/commands/<command>.md
    com.github.copilot/hooks/hooks.json
    com.github.copilot/lsp.json
    skills/<skill>/SKILL.md
    skills/<skill>/references/...
    skills/<skill>/scripts/...
    skills/<skill>/assets/...
    <plugin file>...
    .typeference/bundle.json
```

Each plugin mode is one artifact directory: `manual` emits `<plugin>` and
`pipeline` emits `<plugin>-pipeline`. An artifact renders exactly one mode.

- `plugin.json` holds `$schema`
  (`https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`), `name` (the
  artifact name), `version` (the owning package's version), and `description`
  (the plugin's), then, each when the plugin declares it, `author` (`name`,
  `email`, `url`), `homepage`, `repository`, `license`, and `keywords`, in
  that order.
- `mcp.json` is emitted when a shipped skill requires a server in the
  artifact's mode. It holds `$schema`
  (`https://agent-plugins.org/schemas/1.0.0/mcp.schema.json`) and `mcpServers`,
  one entry per required server keyed by server name and sorted. A `stdio`
  entry holds `type` (`stdio`), `command`, then `args`, `env`, and `cwd` when
  declared; a `streamable-http` entry holds `type` (`streamable-http`), `url`,
  then `headers` when declared.
- `com.github.copilot/agents/<agent>.agent.md` is emitted for each linked
  agent: frontmatter `name` (the agent's identity leaf), `description`, its
  Copilot fields in table order, then its agent-scoped `mcp-servers`; a body of the `# displayName` title and
  the resolved objectives, then, when non-empty, a `## Context` section for
  held documents and a `## Skills` list of the emitted names of the skills the
  agent binds, sorted.
- `skills/<skill>/SKILL.md` is emitted for each shipped skill and instance:
  frontmatter `name` (the emitted name), `description`, then its Copilot fields
  in table order; a body of the skill's instructions for the artifact's mode,
  then, when the skill holds inline documents, a `## Context` section. File
  documents, skill files, and schemas are emitted beside it.
- rules, commands, hooks, and LSP servers are emitted as "Native Copilot
  components" defines.
- each of the plugin's carried files is emitted at its destination.
- `.typeference/bundle.json` (`schemaVersion` 4) records the plugin identity,
  artifact name, mode, version, description, the owning package's provenance,
  the resolved agents, the shipped skills with, for each instance, the template
  identity (`templateId`, empty for a skill that is not an instance) and each
  bound parameter's data identity and canonical values, the shipped
  servers, the shipped rules, commands, hooks, and LSP servers, and the
  plugin's carried files, each with its source and destination. The owning package's provenance is its source digest
  (the build's source digest when the building package owns the plugin, the
  locked digest otherwise) and the locked packages in its dependency closure,
  in lock order.

Frontmatter strings are double-quoted, escaping `\` and `"`. Boolean fields are
unquoted `true` or `false`. List fields are block sequences of double-quoted
strings.

A plugin artifact is therefore a function of its plugin, its mode, its owning
package, and that package's locked dependency closure. Nothing else in the
build, including the building package's own identity or other packages it
ships, reaches an artifact's bytes.

Across a build, every emitted name denotes one thing: an emitted skill name one
skill or instance, a custom agent name one agent, a plugin artifact name one
plugin mode, and a server name one server document. The rule spans packages: it
covers every plugin the build ships, whichever package owns it. If two distinct
resources collapse to one emitted name, compilation fails rather than
overwriting either.

The target root holds `.typeference/build.json` (`schemaVersion` 3: target,
the build's source digest, each artifact's plugin identity, mode, path,
owning package source digest, and directory digest, and each marketplace
file's path, source, and file digest) and `.typeference/compatibility.json`
(`schemaVersion` 1). A file digest is `sha256:` and the SHA-256 of the file's
content as `typeference-directory-v1` reads it. When the manifest declares
`marketplace`, the root also holds `.github/plugin/marketplace.json`: `name`;
`owner` with `name`; `metadata` with the building package's `version`; and
`plugins`, one entry per artifact with `name`, `source` (`./<artifact>`),
`description`, `version` (the owning package's), then the plugin's metadata
members as `plugin.json` orders them; and each marketplace file at its
destination. The target directory is therefore publishable as a marketplace
repository root. Every file in it is an index file build derives, or is
covered by a digest in `build.json`: an artifact's files by its directory
digest, and each marketplace file by its file digest.

The compatibility report lists, for each mode, every capability whose emitted
skills across the build's artifacts come from more than one implementation,
with each member's artifact and skill name. A skill's implementation is the
template it instantiates when it is an instance, and the skill itself
otherwise: instances of one template are distinguished by their data by design
and never compete with each other. Plugins that ship members of one family from
different implementations, such as a team's specialization beside the shared
skill, compete for the same requests when installed together.

Build never emits repository or enterprise settings. Where a marketplace repository lives, and which
repositories enable which plugins, are deployment facts.

## Organization marketplaces

An organization publishes one marketplace, but its agents and skills are
authored in many repositories. The marketplace is therefore the build of one
package, the **marketplace package**, whose manifest lists the plugins of its
direct dependencies:

```text
---
schemaVersion: 7
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

A manifest entry `<package>:<path>` names a plugin that the direct dependency
`<package>` lists in its own manifest; the entry denotes the identity derived
from that package, its declared version, and the path. Naming a package the
manifest does not declare, a path that is not a `.plugin.tfer` document, or a
plugin that the package's manifest does not list is an error. A package ships a
dependency's plugin only when its manifest lists it.

Because the marketplace is one build, every rule of a build holds across the
whole marketplace, whichever package owns each plugin:

- every emitted skill name, custom agent name, plugin artifact name, and MCP
  server name denotes one thing (see "Build targets"). Two packages that ship different
  skills, agents, or plugins under one name fail the build, naming both;
- a skill that several plugins ship is one skill, emitted identically in each
  for a given mode, so installing several plugins that carry it is harmless;
- the compatibility report covers every plugin in the marketplace;
- the locked graph holds one version of each package, and restore rejects a
  graph that would require two (see "Packages, restore, and lockfiles"). Two
  teams that depend on different versions of a shared package cannot both
  ship until one of them moves.

A plugin's artifacts carry its owning package's version and provenance, so a
change to one package's pin changes only that package's artifacts, the
marketplace index, the build index, and the compatibility report.

`typeference validate <marketplace> --candidate <package-dir>` checks whether a
package that is not yet published fits the marketplace. It validates the
marketplace package, with its locked graph, after these substitutions:

- the candidate, a version 7 source directory, replaces the locked package of
  the same name, or joins the graph as a direct dependency when the
  marketplace does not yet depend on it;
- every dependency the candidate declares MUST be locked by the marketplace at
  the declared version, and every locked package that declares a dependency on
  the candidate's name MUST declare the candidate's version;
- the plugins validated are those the marketplace lists from other packages,
  plus every plugin the candidate's manifest lists.

Candidate validation writes nothing, and a candidate is never input to build: a candidate reaches the marketplace only as a published, locked
package.

The published marketplace repository holds only compiler output: the
marketplace package's `agent-plugin` target. Publication replaces the repository's contents with that output, so
anything else in the repository is drift. A README, license, or CI workflow the
repository needs is a marketplace file the marketplace package declares, and
reaches the repository through build like everything else. Where the repository lives, who may
write to it, and how hosts are told about it are deployment facts.


## Canonicalization

All accepted text is UTF-8, BOM-free after loading, LF-normalized, and emitted with
one final LF when its artifact shape requires text termination. Paths use `/`.
Stable string ordering is lexicographic by UTF-8 bytes. Package names, document
paths, identities, mode names, slot names, field names, and metadata keys use
restricted ASCII grammars.

Canonical JSON preserves authored schema member order and decimal lexemes
verbatim, uses defined artifact member order, two-space indentation for files,
compact form for embedded schemas, and deterministic escaping. Map-like objects
sort keys.
Repeated restore, build, and pack operations over identical respective
inputs MUST be byte-identical on every platform.


## Import

`typeference import <source> --out <dir> [--name <package>] [--version
<version>] [--plugin <name>] [--lossy]` converts existing GitHub Copilot
customizations into a version 7 package. The source is, in this order of
detection:

1. a skill directory, one that contains `SKILL.md`;
2. a marketplace repository, one that contains `.github/plugin/marketplace.json`
   or `.claude-plugin/marketplace.json` and no plugin manifest. Import does not
   convert a marketplace: it fails, naming the plugin directories the index
   lists so each can be imported, and noting that root files become the
   marketplace package's `files`;
3. a plugin. A `plugin.json` that declares `$schema` is read as an Agent
   Plugins 1.0 plugin (`skills/*/SKILL.md`, `mcp.json`,
   `com.github.copilot/agents/*.agent.md`, and the commands, rules,
   `hooks/hooks.json`, and `lsp.json` under `com.github.copilot/`). Otherwise `plugin.json` or
   `.claude-plugin/plugin.json` is read as a Copilot CLI plugin, whose `agents`
   and `skills` members locate its components (defaults `agents/` and
   `skills/`);
4. a repository: `.github/agents/*.agent.md`, and skills under
   `.github/skills/`, `.agents/skills/`, and `.claude/skills/`.

Import writes:

- a manifest;
- one plugin, `plugins/<plugin>.plugin.tfer`, that links every imported agent
  and skill and carries the source plugin's metadata (see "Plugin metadata");
- for a plugin source, a plugin `files` entry for each file in the plugin
  directory that is not a component import reads, copied to
  `plugin-files/` with its destination preserved. The components are the
  plugin manifest, `mcp.json`, the skill and agent directories, and, for an
  Agent Plugins 1.0 plugin, `com.github.copilot/`. A file in a component
  directory that import does not read is not carried (see below).
  `.typeference/`, which build regenerates, and `.git/` are skipped;
- `agents/<name>.agent.tfer` for each agent, carrying its description, its
  `name` as `displayName` when that differs from its file name, its body as
  objectives, and its recognized Copilot fields;
- `skills/<name>.skill.tfer` for each skill, carrying its description, its
  body as instructions, its recognized Copilot fields, and a `files` entry for
  each file beside `SKILL.md`, copied to `files/<name>/` with its destination
  preserved;
- `servers/<name>.server.tfer` for each `mcp.json` server. A server whose name
  does not satisfy the server name grammar fails the import;
- `commands/<name>.command.tfer` and `rules/<name>.rule.tfer` for each
  command and rule (a rule's `paths` or `applyTo` becomes `paths`),
  `hooks/<event>-<n>.hook.tfer` for each hook entry, and `lsp/<name>.lsp.tfer`
  for each language server, all linked from the plugin.

Imported skills require every imported server, because the source format does
not record which skill uses which server. Authors narrow `requiresServers`
afterwards.

The plugin name is `--plugin`, else the source plugin's name, else the source
directory's name. The package is `--name` and `--version`, defaulting to
`local/<plugin>` and to the source plugin's `version` when that is an exact
semantic version, else `0.1.0`. The plugin's description is the source plugin's,
else that of the only agent, else that of the only skill when there are no
agents, else a sentence naming the plugin.

Import fails closed:

- names are never rewritten. A skill's declared `name` MUST equal its directory
  name, agent and skill names MUST satisfy the Agent Skills grammar, and
  descriptions and skill instructions are required;
- content version 7 cannot represent fails the import and is listed item by
  item: unrecognized frontmatter fields, files beside `SKILL.md` outside
  `references/`, `scripts/`, and `assets/`, server values that contain `${`
  other than the plugin variables, hook or LSP fields version 7 does not
  declare, plugin manifest members other than `name`, `description`,
  `version`, the metadata members, and a legacy plugin's component paths
  (`extensions` among them), metadata values that "Plugin metadata" rejects,
  files in a component directory that are not components, `.tfer` files in a
  plugin directory, and a legacy Copilot CLI plugin's `hooks`, `commands`,
  and `lspServers`.
  With `--lossy`, import proceeds without them and lists each item it dropped;
- a source plugin `version` that is not an exact semantic version is noted,
  `.typeference/` is noted as not imported, and
  `.github/copilot-instructions.md` is noted as not imported.

Import writes only into a new or empty directory, performs no network access,
and validates the written package with the compiler, failing if it does not
validate.

## Diff and security

`typeference diff` compares relative paths and content as
`typeference-directory-v1` defines it: normalized text for valid UTF-8 files,
exact bytes for any other file. Exit code `0`
means identical, `1` changed, and `2` validation/execution failure.

References and skill files MUST resolve beneath an authorized package root.
Package extraction MUST prevent path traversal and overwrite outside its
materialization directory. Logs MUST avoid secrets. Generated instructions and
`allowed-tools` are descriptive, not authorization; hosts remain responsible for
access control and user approval.
