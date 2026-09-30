# TypeFerence Draft Specification

Status: experimental reference draft, amended September 2026 to version 6. A
version 6 source package declares `schemaVersion: 6` in its project manifest.
Its resource documents carry no schema version, kind, or identity field; the
manifest and each document's path supply them. Unsupported fields and schema
versions are errors rather than extension points.

Version 6 changes from version 5 (ADR-0029 through ADR-0033):

1. GitHub Agent Plugins 1.0 is the primary build target (`agent-plugin`). The
   Codex, Copilot, and Cursor adapters are retired; `neutral` remains the
   canonical all-modes bundle.
2. Plugin documents (`.plugin.tfer`) state which agents, profiles, and skills
   ship together. The manifest lists plugins and exports, and source
   membership is the closure of references from them.
3. Identity is derived: kind from the file suffix, identity from the package
   name and the document's path, version from the manifest. References are
   paths.
4. A skill that binds no capability defines one. A skill may extend one base
   skill additively, and may hold context.
5. An agent's body is its objectives. A context document without a
   `contextType` is a value of the built-in `text` type.
6. `description` is routing metadata: required on agents, skills, and plugins,
   and emitted only into routing surfaces, never into instruction bodies.
7. Scalar typing is schema-directed: the declared type of a field, not the
   spelling of a value, decides what an unquoted scalar means.
8. `typeference import` converts existing GitHub Copilot customizations into
   version 6 source.

Version 5 and earlier sources are archival (see "Archival languages").

## Purpose and pipeline

TypeFerence defines a closed, deterministic source language for composing agent
definitions and compiling them into coherent target artifacts. Its primary
output is a marketplace of GitHub Agent Plugins: shared agents and skills that
people install once and use in every repository and in automated runs. It
separates four operations:

1. **restore** resolves declared source-package dependencies into a locked local
   package tree;
2. **build** resolves typed resources and emits deterministic, unlinked target
   packages without network access or deployment state;
3. **link** validates external deployment bindings and materializes runnable
   host-native configuration;
4. **publish/run** performs environment-specific side effects outside the
   deterministic language core.

The output of each earlier phase is complete input to the next. Build never
implicitly restores. Link may fulfill declared runtime imports, but MUST NOT
change instructions, contracts, context values, composition, selected
implementations, or source identity.

`typeference import` (see "Import") is an authoring aid outside this pipeline:
it writes new source, never artifacts.

TypeFerence does not define an inference runtime, package registry server, secret
manager, model credentials, external tool implementation, or deployment
orchestrator.

## Identity and addressing

**Identity is source; address is deployment.** Package names, versions,
publisher identity, document paths and the identities derived from them,
dependency declarations, and the lock graph are source. Endpoints, commands,
credentials, environment variables, host paths, registry addresses, marketplace
repository addresses, and runtime process details are deployment.

Deployment metadata MUST NOT occur in a source manifest and MUST NOT participate
in resource embedding, composition, skill dispatch, target compilation, source
package identity, or unlinked target identity. Feed URLs and credentials likewise
identify where bytes can be retrieved, not what those bytes mean, and MUST NOT
appear in a lockfile or source digest.

## Project manifest

A version 6 source root contains `typeference.tfer`:

```text
---
schemaVersion: 6
name: helio/works
version: 1.4.0
publisher: helio.example
marketplace:
  name: helio-agents
  owner: Helio Platform
dependencies:
  helio/foundations: 3.1.0
plugins:
  - plugins/payments.plugin.tfer
exports:
  - skills/repository-status.skill.tfer
---
```

The manifest is written in the frontmatter grammar and has no body.
`schemaVersion` is the unquoted token `6`. `name` is a lowercase package name of
two or more `/`-separated segments, each matching `[a-z0-9][a-z0-9.-]*`; the
name `typeference/builtin` and every name beneath it are reserved. `version` and
every dependency version are exact semantic versions (`MAJOR.MINOR.PATCH` with
an optional prerelease); range solving is not defined. A package cannot depend
on itself.

- `publisher` is optional stable publication identity.
- `marketplace` is optional. When present, `name` is at most 64 characters of
  lowercase letters, digits, `.`, and `-`, starting and ending with a letter or
  digit, and `owner` is a required non-empty display name. Build then emits the
  marketplace index (see "Build targets"); it never invents either value.
- `plugins` lists the package's plugin documents (`.plugin.tfer` paths).
- `exports` lists the documents, other than plugins, that other packages may
  reference.

A manifest lists at least one plugin or export and names each at most once.
Unknown fields, deployment fields, feed addresses, and credentials are errors. A
root that contains both a version 6 manifest and the retired `typeference.yaml`
is an error. Product entry points reject a source root whose manifest is absent
or does not declare `schemaVersion: 6`, naming this specification.

## Documents, kinds, and identity

A version 6 source document is a `.tfer` file whose suffix is its kind:

| Suffix | Kind | Role |
| --- | --- | --- |
| `.agent.tfer` | agent | a concrete composition: identity, objectives, held context, skills |
| `.profile.tfer` | profile | a reusable, possibly abstract composition |
| `.interface.tfer` | interface | a structural contract over slots and capabilities |
| `.capability.tfer` | capability | a public method contract shared by unrelated skills |
| `.skill.tfer` | skill | a model-fulfilled capability implementation, emitted as `SKILL.md` |
| `.tool.tfer` | tool | a declared runtime import fulfilled at link time |
| `.contexttype.tfer` | contextType | a named structural type for context values |
| `.context.tfer` | context | a compile-time value of a context type |
| `.plugin.tfer` | plugin | an installable package: the agents, profiles, and skills that ship together |

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

The reserved package `typeference/builtin` holds built-in resources. Version 6
defines one, the context type `typeference/builtin/text@1.0.0` (see "Native
context type language"). Built-in resources are never written as references;
they are what a document means when it omits a type.

## References

A reference names a document by path:

- `skills/review.skill.tfer` names a document in the same package;
- `helio/foundations:skills/review.skill.tfer` names a document in a direct
  dependency. The package MUST be declared in the manifest's `dependencies`;
  the reference denotes the identity derived from that package, its declared
  version, and the path; and that package MUST export the document. Qualifying
  a reference with the referencing package's own name is an error.

Each reference field accepts the kinds listed for it below, checked by suffix;
a reference to any other kind is an error. Plugins are listed only by the
manifest and are never referenced. A reference list names each document at most
once.

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

A body is typed by its document's kind:

- a unimodal skill's body is its instructions. A multimodal skill keeps each
  rendering under `variants` and MUST NOT have a body;
- a context's body is its typed text body;
- an agent's body is its objectives (see "Agents and objectives").

Every other kind, and the manifest, rejects a non-whitespace body.

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
- a decimal field accepts an unquoted JSON number token
  (`-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?`), preserved verbatim:
  `0.50` and `0.5` are distinct values because they are distinct bytes.
  Decimals carry no arithmetic semantics;
- a quoted or block scalar is always a string. It never satisfies a boolean,
  integer, or decimal field;
- a list field accepts `[]` and a mapping field accepts `{}` as empty; null
  is empty for either.

No implicit resolution anywhere chooses a type from a value's spelling.
Floating-point numbers do not exist.

### Field classification rule

A frontmatter field or body is exactly one of:

1. **typed context**: a declared context reference, slot value, or typed body.
   Skill instructions, context bodies, and agent objectives are typed bodies;
   objectives are a value of the built-in text type;
2. **reference**: a path to a document, or a structural pointer such as a slot
   or mode name;
3. **naming and routing metadata**: `displayName` and `description`,
   single-line strings with no control characters or line separators. An
   emitter places `description` only in routing surfaces: `SKILL.md` and
   custom agent frontmatter, `plugin.json`, marketplace entries, the neutral
   skill index, bundle metadata, and publication metadata. `displayName` titles
   an agent's instructions and labels a context section. Neither carries prose
   into an instruction body;
4. **inert metadata**: human-facing documentation that provably never reaches
   model-facing output, such as comments and `description` on a kind that no
   emitter renders.

Nothing else exists. A field whose value reaches model-facing output MUST NOT be
an untyped scalar or list of scalars. The closed per-kind field tables enforce
this rule: any field outside them is an error, and no field inside them carries
free behavioral prose except typed bodies.

### Field tables

| Kind | Fields |
| --- | --- |
| agent, profile | `displayName`, `description`, `embeds` (profiles or agents), `context` (contexts), `slots` (slot name to context), `allowedContextTypes` (context types), `skills` (bindings) |
| interface | `displayName`, `description`, `embeds` (interfaces), `requiresSlots` (slot name to context type), `requiresCapabilities` (capabilities or skills) |
| capability | `displayName`, `description`, `visibility`, `inputSchema`, `outputSchema` |
| skill | `displayName`, `description`, `binds` (capability), `extends` (skill), `sealed`, `inputSchema`, `outputSchema`, `requiresContextTypes` (context types), `requiresTools` (tools), `context` (contexts), `variants` |
| tool | `displayName`, `description`, `inputSchema`, `outputSchema` |
| contextType | `displayName`, `description`, `embeds` (context types), `fields`, `body` |
| context | `displayName`, `description`, `contextType` (context type), `values` |
| plugin | `description`, `agents` (agents), `profiles` (profiles), `skills` (skills), `modes` |

`description` is required on agents, skills, and plugins, and is at most 1024
characters. Schemas are JSON Schema documents written as string values and
canonicalized as JSON. `sealed` is a boolean.

## Plugins

A plugin is the solution file for one installable package. It links; it does
not compose:

```text
---
description: Payments team agent and skills.
agents:
  - agents/payments-repo-agent.agent.tfer
profiles:
  - profiles/review-kit.profile.tfer
skills:
  - skills/changelog.skill.tfer
modes:
  - manual
  - pipeline
---
```

A plugin links at least one agent, profile, or skill. A plugin that links no
agents is a skills pack. `modes` lists `manual`, `pipeline`, or both, each at
most once, and defaults to `manual`.

A plugin ships:

- each linked agent, and every skill that agent resolves to after
  composition;
- for each linked profile, the profile's resolved skills. The profile MUST be
  complete, binding every capability it requires, and MUST hold no context,
  directly or through embedding: without an agent there is nowhere to deliver
  it;
- each linked skill, after flattening its extension chain.

A skill reached more than once ships once.

A plugin's name is its identity leaf. It MUST satisfy the Agent Plugins name
grammar: at most 64 characters of lowercase letters, digits, `.`, and `-`,
starting and ending with a letter or digit, with no `--` or `..`. When the
plugin lists `pipeline`, `<name>-pipeline` MUST satisfy it as well. For each
plugin and each of its modes:

- each linked agent's name, its identity leaf, MUST satisfy the Agent Skills
  name grammar `[a-z0-9]+(-[a-z0-9]+)*` in at most 64 characters, so that it is
  a clean custom agent name;
- each shipped skill's emitted name, its identity leaf, MUST satisfy the same
  grammar, and no two shipped skills may share an emitted name;
- each shipped multimodal skill MUST define a variant for the mode;
- each linked agent's context requirements MUST hold in the mode;
- each skill shipped through `skills` or `profiles` MUST be independent in the
  mode (see "Skill-held context and independence").

## Agents and objectives

An agent is identity (`description`, `displayName`), objectives (its body), and
composition (`embeds`, `skills`, `context`, `slots`, `allowedContextTypes`).
Objectives are typed context, a value of the built-in text type, and render
immediately after the agent's title. Knowledge a whole team needs is context
the agent or its profiles hold. An agent or profile without a `displayName`
takes its identity leaf.

Agents are concrete and MUST fulfill every promoted requirement. Every agent in
the build is resolved and validated; the neutral target emits the package's own
agents and every agent a plugin ships.

## Composition

Agents MAY embed profiles or agents. Profiles MAY embed profiles but MUST NOT
embed agents. An embedding graph MUST NOT contain a cycle.

Resolution proceeds from embedded resources toward the embedding resource:

1. Display name and description belong to their declaring resource.
2. Slots promote by name. The shallowest declaration wins; different
   declarations at the same depth are ambiguous unless declared locally.
3. Held context references append in embedding order and deduplicate in
   first-seen order. Normative behavioral prose exists only as context
   resources of a declared context type; no resource kind carries a norm list
   field. Provenance records each contributing profile or agent for every held
   context.
4. Capability bindings promote by capability identity. The shallowest
   compatible implementation wins. At the same depth, bindings with the same
   resolved implementation and modifier state converge as one member;
   different implementations or incompatible modifier state are ambiguous
   unless bound locally. Provenance retains every contributing path.
5. Allow-lists intersect. A disjoint intersection is an explicit empty set, not
   an unrestricted set.
6. Objectives inherit: an agent's resolved objectives are its embedded agents'
   objectives in embedding order, then its own body, each source at most once
   in first-seen order.
7. Every contribution records source-resource provenance.

Profiles may retain abstract required capability bindings. `required` is the
demand side of composition: a binding with no skill declares an obligation
without supplying an implementation. Structural interface satisfaction only
observes the resolved member set and MUST NOT create or fulfill that
obligation.

## Capabilities, skills, and bindings

A capability document defines canonical JSON `inputSchema` and `outputSchema`
for skills that share a contract but not text. A skill that `binds` a
capability takes the capability's schemas; if it declares schemas, they MUST
equal the capability's byte-for-byte after canonicalization.

A skill that neither binds nor extends defines an **implied capability**,
identified by the skill's own identity, with the skill's schemas and `internal`
visibility. An extension shares its base's capability (see "Skill extension").
A capability reference, in a binding's `capability` or an interface's
`requiresCapabilities`, names a capability document or a skill; naming a skill
denotes that skill's capability.

A capability's `visibility` is `internal` by default or `exposed`. Visibility is
orthogonal to composition: internal and exposed capabilities both promote,
participate in ambiguity checks, and satisfy structural interfaces. Only exposed
capabilities appear on public callable surfaces such as linked A2A Agent Cards
and callable ARD projections. Exposure follows the capability through
embedding. An implied capability is never exposed; exposing a contract requires
a capability document.

A `skills` entry in an agent or profile is a skill path or a mapping:

```text
skills:
  - skills/repository-status.skill.tfer
  - skill: skills/safety-review.skill.tfer
    required: true
    sealed: true
  - capability: capabilities/audit.capability.tfer
    required: true
```

A mapping takes `skill`, `capability`, `required`, and `sealed`. An entry that
names a skill binds that skill's capability; if it also names a capability, the
skill MUST implement it. An entry without a skill MUST name a capability and set
`required: true`, and MUST NOT be sealed: sealing protects a supplied
implementation, not an absent one. A concrete required-and-sealed binding is
valid.

Binding a skill whose capability an embedded layer already binds rebinds that
capability under rule 4 of "Composition". Rebinding or suppressing a promoted
sealed binding is an error. Presence and mutability remain independent:
`required` demands that concrete agents contain a binding, while `sealed`
controls whether a supplied binding may be replaced.

## Skill extension

A skill MAY name exactly one base skill in `extends`. Chains are allowed; a
cycle is an error. A skill declares `binds` or `extends`, never both.

- **Same contract.** An extension takes its base's capability and schemas. It
  may restate its base's schemas; declaring different schemas is an error.
- **Sealed skills.** A skill with `sealed: true` MUST NOT be extended.
  Binding-level `sealed` keeps its meaning; because an extension shares its
  base's capability, binding an extension where a sealed binding of its base
  was promoted is a rebind error.
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
- **Requirements accumulate.** `requiresTools`, `requiresContextTypes`, and
  held `context` are the base's followed by the extension's, deduplicated in
  first-seen order; a mode's variant requirements accumulate the same way.
- **Own identity and routing.** An extension has its own identity, emitted
  name, and required `description`.

## Skill-held context and independence

A skill MAY hold context (`context`), which ships with its `SKILL.md`. The
context types a skill requires, `requiresContextTypes` plus those of the
selected variant, are satisfied by context the skill holds and, where the skill
is bound by an agent, by context the agent holds.

A skill is **independent** in a mode when its own held context satisfies every
context type it requires in that mode; otherwise it is **agent-dependent**.
Independence is derived, never declared. An agent-dependent skill ships only
through an agent that holds satisfying context; shipping it through a plugin's
`skills` or `profiles` is an error. Invoked directly outside its agent, as a
slash command, an agent-dependent skill lacks that context; this is documented
behavior, not a hidden failure.

A skill's emitted text is a function of the skill (its flattened extension
chain and held context) and the mode only. Agent-held context never enters a
`SKILL.md`; it renders in the agent's file. An emitted skill name therefore
denotes one body per mode.

## Invocation modes

A skill declares instructions (its body) or `variants`, never both. Mode names
match `[a-z][a-z0-9-]*`. Base requirements apply to every mode; variant
requirements are additive:

```text
---
description: Report repository status.
requiresTools:
  - tools/repository.tool.tfer
variants:
  manual:
    instructions: Explain the evidence.
  pipeline:
    instructions: Emit strict JSON.
    requiresTools:
      - tools/build-signals.tool.tfer
---
```

A variant takes `instructions` (required), `requiresContextTypes`, and
`requiresTools`. For mode `m`, effective requirements are `base ∪ variant[m]`.
Compilation MUST preserve the mapping and MUST NOT flatten all variants into
one universal requirement set. The neutral target carries every rendering; an
agent-plugin artifact renders exactly one mode. Link selects or validates the
modes requested by deployment and fails if any selected mode's requirements are
unfulfilled.

Variants may change instructions and add context and tool requirements. They
MUST NOT change the bound capability or its schemas.

## Native context type language

Context types use the TypeFerence type language, not embedded JSON Schema:

```text
---
fields:
  owner:
    type: string
    required: true
  participants:
    type: list<string>
    required: true
  governed:
    type: boolean
    default: false
  attributes:
    type: map<string>
body:
  type: text
  required: false
---
```

Version 6 supports `string`, `text`, `boolean`, `integer`, `decimal`,
`list<T>`, `map<T>`, and references to named context types. A field's `type` is
a **type expression**: a scalar constructor name, a parameterized composite
(`list<string>`, `map<string>`, `list<context-types/team.contexttype.tfer>`), or
the path of a named context type. Type expressions are not values; the value
typing rules do not apply to them. A field may be `required` and may declare a
type-correct `default`. There is no `number` constructor; a field declares
`integer` or `decimal`, and `number` is an error that says so. Unknown
constructors and unsupported constraints such as unions, `oneOf`, regular
expressions, arbitrary `$ref`, or open properties are errors. A context type's
`body` takes `type: text` and `required`.

A context declares its type and values:

```text
---
contextType: context-types/team.contexttype.tfer
values:
  owner: payments-platform
  participants:
    - Ari
    - Sam
---
Stable prose carried as the typed body.
```

Values follow the scalar typing rules against their declared field types.
Missing required fields, unknown fields, wrong shapes, and invalid named-type
references are errors. Defaults are materialized before hashing and emission.
Resolved artifacts preserve the complete canonical value object and body, so
any semantic context change changes target bytes. Named value references are
acyclic and may target field-only context types; a context type with a required
body cannot be embedded as an inline field value.

A context that declares no `contextType` is a value of the built-in
`typeference/builtin/text@1.0.0`, which has no fields and a required text body:

```text
---
displayName: Reply style
---
Replies are short and lead with the answer.
```

Context types MAY `embed` other context types to refine them. Type satisfaction
is nominal: a context satisfies its declared type and the transitive set of base
types named by `embeds`; an unrelated type with the same fields does not satisfy
that relationship. The declared member shape is checked structurally.

Field redeclaration must preserve the original type and may only strengthen
optional to required. Identical inherited declarations deduplicate. Sibling bases
that contribute conflicting defaults or modifier state for the same field are
ambiguous unless the derived context type redeclares that field locally with one
compatible resolution.

Agents and profiles hold context with `context`, and skills hold context with
`context` (see "Skill-held context and independence"). Skills require types
with `requiresContextTypes`. `allowedContextTypes` is a closed whitelist:
omitting it means unrestricted and `[]` means no context is allowed. Allow-lists
intersect through composition without treating the empty result as
unrestricted.

`contextFiles` does not exist. Raw prose is represented honestly as a context
with a text body. Slot values reference context documents, never other
filesystem paths.

Emitted Markdown renders each held context as a section titled by its
`displayName`, or its identity leaf, followed by its text body. Field values are
carried in the bundle's canonical context values.

JSON Schema MAY be emitted as a target interoperability projection of a native
context type. It is not accepted as the source language and never controls
TypeFerence validation semantics.

## Tools as runtime imports

A `tool` is an independent extern declaration: a versioned runtime import with
canonical input/output schemas. It is a dependency used by a skill, not a
code-implemented fulfillment of that skill's capability. It contains no
implementation, command, endpoint, environment-specific scope, or credential
value.

A skill or variant lists tool paths in `requiresTools`. Build verifies that
each declaration exists, is a `tool`, and is structurally valid. It does not
pretend the runtime implementation exists. Link binds each effective required
tool to one provider and remote name in the deployment file and fails closed if
the declaration, binding, provider, or selected mode is missing or malformed.
The deployment binding attests that the named remote callable implements the
exact source tool contract. TypeFerence does not interrogate the provider or
prove its runtime schema, authorization, or cryptographic validity.

The vocabulary is intentionally separate:

- capability: agent behavior contract;
- skill: model implementation of a capability;
- tool: imported runtime callable used by a skill.

An MCP projection may map capabilities and tools to MCP wire-level tools, but
that transport projection does not collapse the source-language categories.

## Packages, restore, and lockfiles

`typeference pack` emits one canonical source package (`.tferpkg`) from a
version 6 source root. It contains:

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
MAY implement filesystem, generic HTTP/JFrog Generic, and Azure Artifacts
transports. Routes select one feed for a namespace; implementations MUST NOT
search every configured feed. Credentials are environment/helper references and
never source.

Build reads only the source root, lockfile, and a supplied materialized package
directory. It verifies locked digests and performs no network access. Missing,
undeclared, conflicting, or corrupt dependencies are errors. A package-qualified
reference MUST name a document its package exports. A dependency's plugin
documents belong to that package's own build and never ship in a dependent's.

## Source membership and digests

The source digest hashes an explicit resource set, never an arbitrary recursive
directory after output has been written. A version 6 package's source members
are:

- the project manifest;
- the lockfile and the root trust file `typeference.trust.tfer`, when present;
- every document in the closure of references from the manifest's plugins and
  exports.

Files outside the closure are not members: they are neither parsed nor hashed.
The set excludes `.git`, `dist`, `bin`, `obj`, restored packages, deployment
files, feed configuration, signature maps, and generated artifacts. Build MUST
reject an output directory nested inside the source root unless the output path
is in a normatively excluded generated directory (`dist`, `bin`, or `obj`). The
source root itself is never a valid output directory.

`typeference-resource-set-v1` sorts normalized source-relative paths by UTF-8 byte
order and hashes, for each file, `path`, NUL, normalized content, NUL using
SHA-256. Target provenance records the root source digest, the ordered locked
dependency digests, and target adapter identity. Release metadata belongs to the
distributed compiler binary, not the reproducible source-derived artifact.

`typeference-directory-v1` remains the target-directory digest: recursively sort
forward-slash paths, then hash `path`, NUL, normalized text, NUL. It applies only
to already-defined artifact directories, not source identity.

## Build targets

Build emits deterministic **unlinked** targets beneath `<out>/<target>/`.
Version 6 defines two:

- `agent-plugin`: GitHub Agent Plugins 1.0 packages and their marketplace
  index;
- `neutral`: the canonical all-modes bundle for each emitted agent.

`--target all`, the default, builds both. A request for a retired target
(`codex`, `copilot`, `cursor`) fails with a diagnostic naming ADR-0029. Each
target MUST represent every portable resolved field or emit a diagnostic.
Host-native active runtime configuration is not a build artifact.

An unlinked target contains instructions, skills, bundle metadata, mode
requirements, tool imports, complete compiled context values, provenance, and
link-requirements manifests. It MUST NOT contain invented endpoints, commands,
unresolved `${TOKEN}` text in active configuration, or environment defaults.

Across a build, every emitted name denotes one thing: an emitted skill name one
skill (among the skills the build's agents bind and its plugins ship), a custom
agent name one agent, a plugin artifact name one plugin mode, and a neutral
agent directory one agent. If two distinct resources collapse to one emitted
name, compilation fails rather than overwriting either.

### The agent-plugin target

```text
agent-plugin/
  .github/plugin/marketplace.json
  .typeference/build.json
  .typeference/compatibility.json
  <artifact>/
    plugin.json
    com.github.copilot/agents/<agent>.agent.md
    skills/<skill>/SKILL.md
    .typeference/bundle.json
    .typeference/link.json
```

Each plugin mode is one artifact directory: `manual` emits `<plugin>` and
`pipeline` emits `<plugin>-pipeline`. An artifact renders exactly one mode.

- `plugin.json` holds exactly `$schema`
  (`https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`), `name` (the
  artifact name), `version` (the package version), and `description` (the
  plugin's), in that order.
- `com.github.copilot/agents/<agent>.agent.md` is emitted for each linked
  agent: frontmatter `name` (the agent's identity leaf) and `description`;
  a body of the `# displayName` title and the resolved objectives, then, when
  non-empty, a `## Context` section for held context and a `## Skills` list of
  the emitted names of the skills the agent binds, sorted.
- `skills/<skill>/SKILL.md` is emitted for each shipped skill: frontmatter
  `name` (the emitted name, the skill's identity leaf) and `description`; a
  body of the skill's instructions for the artifact's mode, then, when the
  skill holds context, a `## Context` section.
- `.typeference/bundle.json` (`schemaVersion` 1) records the plugin identity,
  artifact name, mode, version, description, the resolved agents, and the
  shipped skills.
- `.typeference/link.json` (`schemaVersion` 2) records the plugin identity,
  `kind: plugin`, target, mode, source digest, locked dependencies, the modes
  the shipped skills define, and the tool imports: every shipped skill's base
  tools under mode `*`, plus the artifact mode's variant tools.

Frontmatter descriptions are double-quoted, escaping `\` and `"`.

The target root holds `.typeference/build.json` (`schemaVersion` 2: target,
source digest, and each artifact's plugin identity, mode, path, and directory
digest) and `.typeference/compatibility.json` (`schemaVersion` 1). When the
manifest declares `marketplace`, it also holds `.github/plugin/marketplace.json`:
`name`; `owner` with `name`; `metadata` with the package `version`; and
`plugins`, one entry per artifact with `name`, `source` (`./<artifact>`),
`description`, and `version`. The target directory is therefore publishable as
a marketplace repository root.

The compatibility report lists, for each mode, every capability that more than
one distinct emitted skill implements across the build's artifacts, with each
member's artifact and skill name. Such skills form a family, and plugins that
ship different members of one family compete for the same requests when
installed together. Installing one plugin per family at a time avoids that.

Build never emits `mcp.json`, hooks, or repository or enterprise settings.
Where a marketplace repository lives, and which repositories enable which
plugins, are deployment facts.

### The neutral target

For each emitted agent the neutral target writes `<agent>/AGENTS.md`,
`bundle.json`, `provenance.json`, `.typeference/link.json` (`schemaVersion` 1),
and, for each skill, `skills/<skill>/SKILL.md` plus one `SKILL.<mode>.md` per
variant. `SKILL.md` renders unimodal instructions or, for a multimodal skill, a
default variant: `pipeline`, else `manual`, else `a2a`, else the first mode in
byte order. The target root holds `.typeference/build.json` (`schemaVersion` 1).

`AGENTS.md` renders the `# displayName` title and the resolved objectives, then,
when non-empty, a `## Context slots` list, a `## Context` section, and an
`## Available skills` index with one line per skill: its dispatch name
(`<agent leaf>.<capability leaf>`), its `SKILL.md` path, and its description.
The skill index is the neutral bundle's routing surface. The agent's own
description is not rendered in `AGENTS.md`.

## Deployment files and link

Deployment is supplied explicitly outside the source package:

```text
schemaVersion: 1
environment: staging
artifacts:
  helio/works/agents/payments-repo-agent@1.4.0:
    modes: [manual, a2a]
  helio/works/plugins/payments@1.4.0:
    modes: [manual, pipeline]
providers:
  repository-signals:
    kind: mcp
    transport: stdio
    command: repository-signals-mcp
    args: [--bundle, "{bundle}"]
toolBindings:
  helio/works/tools/repository-signals@1.4.0:
    provider: repository-signals
    remoteName: repository_signals
agentEndpoints:
  helio/works/agents/payments-repo-agent@1.4.0:
    a2aUrl: https://agents.example/payments
```

The deployment schema is closed. Commands and arguments are distinct string
values. Stdio environment entries are same-name forwarding references
(`NAME: { fromEnvironment: NAME }`), never secret values. HTTP bearer
authentication uses `bearerTokenEnvironment` and likewise names an environment
variable rather than its value. URLs MUST be absolute HTTPS URLs without
embedded credentials.

Deployment-provider substitution is closed and adapter-directed. It applies
only where link materializes host-native configuration, which in version 6 is a
plugin artifact's `mcp.json`. There, in a stdio MCP provider's `args` only,
every `{bundle}` occurrence is replaced with
`${PLUGIN_ROOT}/.typeference/bundle.json`, the Agent Plugins expansion for the
installed plugin's bundle. It is not replaced in `command`, environment
bindings, HTTP provider fields, or compiled files. No other token has special
meaning in deployment schema version 1; other argument text remains literal.

`typeference link <built-target> --deployment <file> --out <dir>`:

1. verifies the unlinked target digest against its build index;
2. selects the artifact modes;
3. validates every effective tool import and endpoint requirement;
4. emits a canonical binding manifest per artifact that preserves the
   artifact's sorted selected-mode set and each effective source tool ID,
   requirement mode, provider, and provider-level remote name;
5. structurally serializes host-native configuration;
6. records deployment-file and materialized-artifact digests separately.

A neutral artifact is selected by agent identity. It carries every rendering;
link records which renderings deployment selected even when the artifact imports
no tools, and a multimodal neutral artifact requires at least one selected mode.
Linking a neutral artifact writes its binding manifest and, when the deployment
declares an endpoint, its A2A Agent Card; it materializes no other host
configuration.

A plugin is selected by plugin identity, and its artifacts link together: the
deployment MUST select exactly the modes the plugin was built in. Each artifact
validates the tool imports of its own mode. Link then writes
`<artifact>/mcp.json` for the providers those imports use, when there are any:
`$schema` (`https://agent-plugins.org/schemas/1.0.0/mcp.schema.json`) and
`mcpServers`, one server per provider named by its deployment key, sorted.

- A stdio server is `{"type": "stdio", "command": ..., "args": [...]}`. Its
  command MUST be a bare executable name matching `[A-Za-z0-9_][A-Za-z0-9._+-]*`
  or a plugin-relative `./` path without `..`.
- An HTTP server is `{"type": "streamable-http", "url": ...}`.

An Agent Plugins configuration may not depend on inherited environment
variables, and `mcp.json` never contains a secret. Link therefore fails closed
when a plugin's provider declares stdio `environment` bindings or
`bearerTokenEnvironment`.

A2A Agent Cards are linked from neutral artifacts only. For a multimodal agent,
the deployment MUST select `a2a`; only exposed capabilities whose concrete skill
is unimodal or defines an `a2a` variant are advertised.

The same source and unlinked target linked for staging and production MUST retain
identical source and unlinked-target digests. Only the linked artifact and
deployment provenance may differ.

Linked output preserves the input integrity index as historical
`unlinked-build.json`; it MUST NOT leave that index named as though it described
the mutated linked tree. Link provenance records a digest for each linked
artifact directory from outside that directory, avoiding a self-digest cycle.
Its `schemaVersion` is 2, listing each artifact's identity, mode, path, and
digest, when the target is `agent-plugin`, and 1, listing agent identity, path,
and digest, when it is `neutral`.

Link may create an absent output directory or populate an existing empty one. It
MUST reject an existing non-empty output unless the directory root contains a
valid `.typeference/link-provenance.json` written by a completed TypeFerence
link. A previously linked output may be recursively replaced; an unowned
directory, file, filesystem root, or path that contains or is contained by the
unlinked input MUST fail before output deletion or writing. The provenance file
is an ownership marker for safe replacement, not a cryptographic trust claim.

## ARD and callable publication

ARD publication consumes build and optional link artifacts. With `--emit-ard`,
build writes a catalog entry for the source package and one for each built
artifact: each neutral agent bundle and each plugin artifact
(`urn:air:<publisher domain>:typeference:agent-plugin:<artifact>`). Source-package
and unlinked-target entries are deterministic. A2A Agent Cards, MCP server
cards, and other callable publications require an explicitly linked
endpoint/provider; TypeFerence MUST NOT invent an address from publisher
identity.

Publisher identity may participate in stable URNs and trust identity. Endpoint
addresses do not. Publication remains an explicit edge operation and may perform
network I/O only when requested.

## Trust metadata

A source root MAY contain `typeference.trust.tfer` or select one explicitly. Trust
metadata is declarative and participates in source-package publication, not
behavioral resolution. TypeFerence MUST NOT dereference identity, attestation,
provenance, policy, or key URIs; sign; verify cryptographic validity; or resolve
keys.

Externally produced compact detached JWS strings MAY be imported from a signature
map outside the source root. Unknown identifiers are errors.
`signatureIntent.required` fails closed unless an explicit unsigned-staging option
is used solely to emit payloads for an external signer. The signature map MUST
remain outside the source root to avoid a digest/signature cycle.

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
Repeated restore, build, pack, and link operations over identical respective
inputs MUST be byte-identical on every platform.

## Import

`typeference import <source> --out <dir> [--name <package>] [--version
<version>] [--plugin <name>] [--lossy]` converts existing GitHub Copilot
customizations into a version 6 package. The source is, in this order of
detection:

1. a skill directory, one that contains `SKILL.md`;
2. a plugin. A `plugin.json` that declares `$schema` is read as an Agent
   Plugins 1.0 plugin (`skills/*/SKILL.md`,
   `com.github.copilot/agents/*.agent.md`). Otherwise `plugin.json` or
   `.claude-plugin/plugin.json` is read as a Copilot CLI plugin, whose `agents`
   and `skills` members locate its components (defaults `agents/` and
   `skills/`);
3. a repository: `.github/agents/*.agent.md`, and skills under
   `.github/skills/`, `.agents/skills/`, and `.claude/skills/`.

Import writes a manifest; one plugin, `plugins/<plugin>.plugin.tfer`, that
links every imported agent and skill; `agents/<name>.agent.tfer` for each
agent, carrying its description, its `name` as `displayName` when that differs
from its file name, and its body as objectives; and `skills/<name>.skill.tfer`
for each skill, carrying its description and its body as instructions. The
plugin name is `--plugin`, else the source plugin's name, else the source
directory's name. The package is `--name` and `--version`, defaulting to
`local/<plugin>` and `0.1.0`. The plugin's description is the source plugin's,
else that of the only agent, else that of the only skill when there are no
agents, else a sentence naming the plugin.

Import fails closed:

- names are never rewritten. A skill's declared `name` MUST equal its directory
  name, agent and skill names MUST satisfy the Agent Skills grammar, and
  descriptions and skill instructions are required;
- content version 6 cannot represent fails the import and is listed item by
  item: frontmatter fields other than `name` and `description`, files beside
  `SKILL.md`, `mcp.json`, hooks, commands, rules, LSP configuration, and a
  Copilot CLI plugin's `hooks`, `mcpServers`, `commands`, and `lspServers`.
  With `--lossy`, import proceeds without them and lists each item it dropped;
- plugin metadata other than `name`, `description`, and `version` is noted as
  not carried, and `.github/copilot-instructions.md` is noted as not imported.

Import writes only into a new or empty directory, performs no network access,
and validates the written package with the compiler, failing if it does not
validate.

## Diff and security

`typeference diff` compares relative paths and normalized content. Exit code `0`
means identical, `1` changed, and `2` validation/execution failure.

References MUST resolve beneath an authorized package root. Package extraction
MUST prevent path traversal and overwrite outside its materialization directory.
Logs MUST avoid secrets and MCP stdio stdout. Tool annotations and generated
instructions are descriptive, not authorization; hosts remain responsible for
access control and user approval.

## Archival languages

Version 5 and earlier sources (`schemaVersion: 5` and older resources with
`schemaVersion: 2` manifests) are archival. Product entry points (the CLI, the
language server, the playground, and pack) never read them. The conformance
runner alone reproduces the archival corpora, selecting an archival loader per
fixture (`legacy-v5`, `legacy-v3`) and building only the targets version 6
defines: archival fixtures reproduce their recorded neutral output
byte-for-byte, and their ARD catalogs list only neutral bundles. The version 5
rules are this document as of commit `606742d`.
