# TypeFerence Draft Specification

Status: experimental reference draft, amended August 2026. Typed resources use
`schemaVersion: 5`; project manifests use `schemaVersion: 2`. Version 5 is a
deliberate closure of the source language. Unsupported fields and schema versions
are errors rather than extension points.

Version 5 changes from version 4:

1. `.tfer` (frontmatter-plus-body) is the only accepted source format; bare
   YAML sources are no longer valid.
2. Frontmatter and manifest syntax are TypeFerence's own closed indentation
   grammar (see "Source document formats"); YAML is no longer the surface
   parser.
3. `workingNorms` no longer exists on any resource kind; behavioral prose
   reaches a model only as typed context (see "Composition" and "Native
   context type language").

## Purpose and pipeline

TypeFerence defines a closed, deterministic source language for composing agent
definitions and compiling them into coherent target artifacts. It separates four
operations:

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

TypeFerence does not define an inference runtime, package registry server, secret
manager, model credentials, external tool implementation, or deployment
orchestrator.

## Identity and addressing

**Identity is source; address is deployment.** Resource IDs, package IDs,
versions, publisher identity, dependency declarations, and the lock graph are
source. Endpoints, commands, credentials, environment variables, host paths,
registry addresses, and runtime process details are deployment.

Deployment metadata MUST NOT occur in a source manifest and MUST NOT participate
in resource embedding, composition, skill dispatch, target compilation, source
package identity, or unlinked target identity. Feed URLs and credentials likewise
identify where bytes can be retrieved, not what those bytes mean, and MUST NOT
appear in a lockfile or source digest.

## Project manifest

A source root MAY contain `typeference.tfer`:

```text
schemaVersion: 2
name: marathon/payments-agents
version: 1.0.0
publisher: marathon.example
dependencies:
  marathon/enterprise-foundations: 3.1.0
```

`name`, `version`, and every dependency version are required when the manifest is
present. Versions are exact semantic versions in version 5; range solving is not
defined. Dependency names use the resource namespace grammar. `publisher` is
optional stable publication identity. Unknown fields, deployment fields, feed
addresses, and credentials are errors.

## Resource identity and kinds

A compilation unit contains UTF-8 frontmatter-plus-body documents with
`schemaVersion: 5`, a `kind`, and an `id`. IDs use
`namespace/name@semantic-version`. Supported kinds are:

- `agent`: a concrete, target-emitting composition;
- `profile`: a reusable, possibly abstract composition;
- `interface`: a structural contract over slots and capabilities;
- `capability`: a stable public method contract;
- `skill`: a model-fulfilled capability implementation;
- `tool`: a declared runtime import fulfilled at link time;
- `contextType`: a named structural type for context values;
- `context`: a compile-time value of a context type.

All references are exact resource IDs. A reference resolves either to a document
in the root package or to an export in the committed dependency graph. Searching
feeds, selecting a similarly named resource, or allowing a local resource to
shadow a dependency is forbidden.

## Source document formats

Version 5 accepts exactly one source format: the `.tfer` document, a
frontmatter-plus-body file:

```text
---
schemaVersion: 5
kind: skill
id: acme/skills/example@1.0.0
binds: acme/capabilities/example@1.0.0
---
Model-facing instructions.
```

The opening and closing `---` fences MUST each occupy an exact line. The file
MUST start with the opening fence. The content between the fences is exactly one
mapping in the TypeFerence frontmatter grammar (below) and is subject to the
same closed-field rules as every other part of the language. The text after the
closing fence is the body. A bare `.yaml` resource document is not valid
version 5 source and MUST be rejected with a diagnostic naming the format.

### Frontmatter grammar

The frontmatter grammar is TypeFerence's own closed indentation grammar. Its
syntax is fixed by this specification; it is not YAML and has no YAML resolver
semantics. It supports exactly:

- `key: value` mappings nested by two-space indentation increments;
- flow-free sequences (`- item` lines) at any nesting depth;
- single- or double-quoted strings, which are always strings;
- block scalars for multiline string values: literal `|`, stripped `-|`, and
  an explicit indentation indicator (`|2`). Block-scalar chomping and relative
  indentation follow YAML 1.2's defined semantics, cited here rather than
  reinvented;
- comments: a `#` that starts a line or follows whitespace outside quotes
  begins a comment, which is inert metadata and never reaches artifacts;
- duplicate keys are errors.

Scalar typing is syntactic; no implicit resolution ever chooses a type:

- a quoted scalar is a string;
- an unquoted scalar matching `[0-9]+` (optionally signed) is an integer,
  preserved as its written digit string;
- an unquoted scalar matching a decimal number pattern (digits with `.`
  or exponent) is a decimal, canonicalized by preserving the written lexeme
  verbatim; `0.50` and `0.5` are distinct values because they are distinct
  bytes. Decimals carry no arithmetic semantics;
- `true` and `false` unquoted are booleans;
- `null` unquoted is null;
- every other bare-word scalar is an error. There is no implicit typing: a
  value such as `no` or `staging` MUST be quoted to be a string.

The vocabulary of scalars is deliberately small — quoted strings, integers,
decimals, booleans, null — plus the composites mapping, sequence, and named
context types. Integers are arbitrary precision; implementations map them to a
bounded width only where a schema declares one. Floating-point numbers do not
exist in version 5.

Input text is UTF-8, BOM-stripped, and CRLF-normalized to LF before the document
is split. The normalized body is otherwise preserved. Trimming is used only to
decide whether a body is empty and whether a required context body is present.

Only a unimodal `skill` and a `context` may carry a non-whitespace body. For a
unimodal skill the body is `instructions`; a skill MUST NOT declare instructions
both in frontmatter and in its body. A multimodal skill keeps each instruction
rendering under `variants` and MUST have an empty body. A context body is the
typed text body declared by its `contextType`. Every other resource kind rejects
a non-whitespace body.

### Field classification rule

A frontmatter field on any resource kind is exactly one of:

1. **typed context** — its value is a declared context reference, slot value,
   or typed body;
2. **reference** — an exact resource ID or structural pointer into the
   composition graph;
3. **inert metadata** — human-facing documentation that provably never reaches
   model-facing output.

Nothing else exists. A field whose value reaches model-facing output MUST NOT
be an untyped scalar or list of scalars. This rule is enforced by the closed
per-kind field maps: any field outside them is already an error, and no field
inside them carries free behavioral prose except skill instructions and typed
context bodies.

### Migration from YAML sources

Conversion from version 4 dual formats is mechanical and syntax-only:
frontmatter content moves unchanged into the frontmatter grammar (quoting any
scalar that the rules above require quoting); bodies move unchanged. No
semantic rewrite occurs. Manifests (`typeference.tfer`) keep their name and
closed field set but are parsed with the same frontmatter grammar.

## Composition

Agents MAY embed profiles or agents. Profiles MAY embed profiles but MUST NOT
embed agents. An embedding graph MUST NOT contain a cycle.

Resolution proceeds from embedded resources toward the embedding resource:

1. Display name and description belong to their declaring resource.
2. Slots promote by name. The shallowest declaration wins; different
   declarations at the same depth are ambiguous unless declared locally.
3. Held context IDs append in embedding order and deduplicate in first-seen
   order. Normative behavioral prose exists only as context resources of a
   declared `contextType`; no resource kind carries a norm list field.
   Provenance records each contributing profile or agent for every held
   context, preserving the per-contributor path that `workingNorms` previously
   provided.
4. Capability bindings promote by capability ID. The shallowest compatible
   implementation wins. At the same depth, bindings with the same resolved
   implementation and modifier state converge as one member; different
   implementations or incompatible modifier state are ambiguous unless bound
   locally. Provenance retains every contributing path.
5. Allow-lists intersect. A disjoint intersection is an explicit empty set, not
   an unrestricted set.
6. Every contribution records source-resource provenance.

Profiles may retain abstract required capability bindings. Agents are concrete
and MUST fulfill every promoted requirement. `required` is the demand side of
composition: a ref-less required binding declares an obligation without supplying
an implementation. Structural interface satisfaction only observes the resolved
member set and MUST NOT create or fulfill that obligation.

## Interfaces, capabilities, skills, and bindings

Interfaces MAY require typed slots and capability IDs and MAY embed interfaces.
They provide no implementation. In version 5, `requiresSlots` maps each required
slot name to a contextType ID:

```text
requiresSlots:
  repository: acme/context-types/repository-evidence@1.0.0
```

The supplied slot value MUST reference a context resource satisfying that type
or a refinement. Satisfaction is inferred from the resolved member set.

A capability defines canonical JSON `inputSchema` and `outputSchema`. A skill
declares `binds: <capability-id>` and MUST preserve those schemas byte-for-byte
after canonicalization. Its instructions provide the model implementation.

A capability's `visibility` is `internal` by default or `exposed`. Visibility is
orthogonal to composition: internal and exposed capabilities both promote,
participate in ambiguity checks, and satisfy structural interfaces. Only exposed
capabilities appear on public callable surfaces such as linked A2A Agent Cards
and callable ARD projections. Exposure follows the capability through embedding.

Bindings have independent presence and mutability axes:

```text
skills:
  - capability: acme/capabilities/audit@1.0.0
    required: true
  - capability: acme/capabilities/safety@1.0.0
    ref: acme/skills/safety@1.0.0
    required: true
    sealed: true
```

A binding without `ref` MUST be `required: true` and MUST NOT be sealed. Sealing
protects a supplied implementation; it cannot protect an absent one. Therefore
`required: true, sealed: true` without `ref` is a compile error. A concrete
required-and-sealed binding is valid. Rebinding or suppressing a promoted sealed
binding is an error. Presence and mutability remain independent: `required`
demands that concrete agents contain a binding, while `sealed` controls whether a
supplied binding may be replaced.

## Invocation modes

A skill declares either `instructions` or `variants`, never both. Base
requirements apply to every mode; variant requirements are additive:

```text
kind: skill
requiresTools:
  - acme/tools/repository@1.0.0
variants:
  manual:
    instructions: Explain the evidence.
  pipeline:
    instructions: Emit strict JSON.
    requiresTools:
      - acme/tools/build-signals@1.0.0
```

For mode `m`, effective requirements are `base ∪ variant[m]`. Compilation MUST
preserve the mapping and MUST NOT flatten all variants into one universal
requirement set. A target build may carry every mode. Link selects or validates
the modes requested by deployment and fails if any selected mode's requirements
are unfulfilled. A target adapter MAY declare a default mode, but MUST diagnose a
missing default rather than silently substitute another mode.

Variants may change instructions and add context/tool requirements. They MUST NOT
change the bound capability or its schemas.

## Native context type language

Context types use the TypeFerence type language, not embedded JSON Schema:

```text
schemaVersion: 5
kind: contextType
id: acme/context-types/team@1.0.0
fields:
  owner:
    type: string
    required: true
  participants:
    type: "list<string>"
    required: true
  governed:
    type: boolean
    default: false
  attributes:
    type: "map<string>"
body:
  type: text
  required: false
```

Version 5 supports `string`, `text`, `boolean`, `integer`, `decimal`,
`list<T>`, `map<T>`, and exact references to named context types. A field's
`type` member is a **type expression**: a bare scalar constructor name
(`string`, `text`, `boolean`, `integer`, `decimal`), a parameterized composite
written as a quoted string (`"list<string>"`, `"map<string>"`,
`"list<acme/context-types/team@1.0.0>"`), or an exact named context type ID.
Type expressions are not values; the value-scalar rules below do not apply to
them. A field may be `required` and may declare a type-correct scalar or
collection `default`. Unknown constructors and unsupported constraints such as
unions, `oneOf`, regular expressions, arbitrary `$ref`, or open properties are
errors.

Scalar values in context `values` and `default` members follow the same
syntactic typing as the frontmatter grammar: quoted is string, digits are
integer, decimal lexemes are decimals preserved verbatim, unquoted
true/false/null are boolean/null, and any other bare word is an error. No
implicit resolution ever chooses a field's meaning. There is no `number`
(floating-point) constructor; a schema that previously used it declares
`integer` or `decimal`. Integers are arbitrary precision and canonicalized as
their written digit string.
Named value references are acyclic and may target field-only context types;
a context type with a required prose body cannot be embedded as an inline field
value.

A context instance declares its type and values:

```text
schemaVersion: 5
kind: context
id: acme/context/payments-team@1.0.0
contextType: acme/context-types/team@1.0.0
values:
  owner: payments-platform
  participants:
    - "Ari"
    - "Sam"
---
Stable prose carried as the typed body.
```

Values are parsed with the syntactic scalar rules of the frontmatter grammar;
no implicit resolution decides their meaning. Missing required fields, unknown
fields, wrong shapes, and invalid named-type references are errors. Defaults
are materialized before hashing and emission. Resolved artifacts preserve the
complete canonical value object and body, so any semantic context change
changes target bytes.

Context types MAY explicitly `embed` other context types to refine them. Type
satisfaction is nominal: a context satisfies its declared type and the transitive
set of base type IDs named by `embeds`; an unrelated type with the same fields
does not satisfy that relationship. The declared member shape is checked
structurally.

Field redeclaration must preserve the original type and may only strengthen
optional to required. Identical inherited declarations deduplicate. Sibling bases
that contribute conflicting defaults or modifier state for the same field are
ambiguous unless the derived context type redeclares that field locally with one
compatible resolution.

Agents and profiles hold context by resource ID using `context`. Skills require
types using `requiresContextTypes`. `allowedContextTypes` is a closed whitelist;
omitting it means unrestricted and specifying `[]` means no context is allowed.
Allow-lists intersect through composition without treating the empty result as
unrestricted.

`contextFiles` does not exist. Raw prose is represented honestly as
a context type with a required `text` body. Slot values that carry context MUST
reference context resource IDs, never filesystem paths. All referenced files and
resources MUST be explicit members of the source package and resolve beneath its
root.

JSON Schema MAY be emitted as a target interoperability projection of a native
context type. It is not accepted as the source language and never controls
TypeFerence validation semantics.

## Tools as runtime imports

A `tool` is an independent extern declaration: a versioned runtime import with
canonical input/output schemas. It is a dependency used by a skill, not a
code-implemented fulfillment of that skill's capability. It contains no
implementation, command, endpoint, environment-specific scope, or credential
value.

A skill or variant lists exact tool IDs in `requiresTools`. Build verifies that
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

`typeference pack` emits one canonical source package (`.tferpkg`) containing:

- package name and exact version;
- exact dependency declarations;
- sorted exported resource IDs;
- sorted canonical source paths and normalized UTF-8 contents;
- a `sha256:` package digest.

The package envelope is canonical JSON. Paths use `/`, may not be absolute,
contain `..`, or collide after normalization. Files are LF-normalized and BOM-free.
No archive timestamp, host path, feed address, credential, deployment value,
generated target, or VCS data is permitted.

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
undeclared, conflicting, or corrupt dependencies are errors.

## Source membership and digests

The source digest hashes an explicit resource set, never an arbitrary recursive
directory after output has been written. It includes:

- the project manifest and lockfile when present;
- every root resource and context source file accepted by the loader;
- explicitly selected trust configuration;
- referenced source assets defined by a future schema version.

It excludes `.git`, `dist`, `bin`, `obj`, restored packages, deployment files,
feed configuration, signature maps, generated artifacts, and unreferenced files.
Build MUST reject an output directory nested inside the source root unless the
output path is in a normatively excluded generated directory. The source root
itself is never a valid output directory.

`typeference-resource-set-v1` sorts normalized source-relative paths by UTF-8 byte
order and hashes, for each file, `path`, NUL, normalized content, NUL using
SHA-256. Target provenance records the root source digest, the ordered locked
dependency digests, and target adapter identity. Release metadata belongs to the
distributed compiler binary, not the reproducible source-derived artifact.

`typeference-directory-v1` remains the target-directory digest: recursively sort
forward-slash paths, then hash `path`, NUL, normalized text, NUL. It applies only
to already-defined artifact directories, not source identity.

## Build targets

Build emits deterministic **unlinked** packages for neutral, Codex, GitHub
Copilot, and Cursor targets. Each adapter MUST represent every portable resolved
field or emit a diagnostic. Host-native active runtime configuration is not a
build artifact.

Every emitted agent artifact path and callable dispatch name MUST be unique
after target-native naming. If two otherwise distinct resource IDs collapse to
the same target name, compilation fails rather than overwriting either artifact.

An unlinked target contains instructions, skills, native rules, bundle metadata,
mode requirements, tool imports, complete compiled context values, provenance,
and a link-requirements manifest. It MUST NOT contain invented endpoints,
commands, unresolved `${TOKEN}` text in active configuration, or environment
defaults.

## Deployment files and link

Deployment is supplied explicitly outside the source package:

```text
schemaVersion: 1
environment: staging
artifacts:
  marathon/agents/payments@1.0.0:
    modes: [manual, a2a]
providers:
  payments-tools:
    kind: mcp
    transport: stdio
    command: payments-mcp-server
    args: [--bundle, "{bundle}"]
    environment:
      PAYMENTS_TOKEN:
        fromEnvironment: PAYMENTS_TOKEN
toolBindings:
  marathon/tools/repository-signals@1.0.0:
    provider: payments-tools
    remoteName: repository_signals
agentEndpoints:
  marathon/agents/payments@1.0.0:
    a2aUrl: https://agents.example/payments
```

The deployment schema is closed. Commands and arguments are distinct string
values. Stdio environment entries are same-name forwarding references
(`NAME: { fromEnvironment: NAME }`), never secret values. HTTP bearer
authentication uses `bearerTokenEnvironment` and likewise names an environment
variable rather than its value. URLs MUST be absolute HTTPS URLs unless a
transport-specific local-development option explicitly allows otherwise.

Deployment-provider substitution is closed and adapter-directed. In a stdio MCP
provider's `args` only, every `{bundle}` occurrence is replaced with
`.typeference/bundle.json`, the bundle path relative to the linked agent artifact
root. It is not replaced in `command`, environment bindings, HTTP provider
fields, or compiled files. No other token has special meaning in deployment
schema version 1; other argument text remains literal.

`typeference link <built-target> --deployment <file> --out <dir>`:

1. verifies the unlinked target digest;
2. selects the artifact modes;
3. validates every effective tool import and endpoint requirement;
4. emits a canonical binding manifest that preserves the artifact's sorted
   selected-mode set and each effective source tool ID, requirement mode,
   provider, and provider-level remote name;
5. structurally serializes host-native configuration;
6. records deployment-file and materialized-artifact digests separately.

For Codex, local stdio MCP bindings materialize `.codex/config.toml` using TOML
string escaping; HTTP bindings use the native URL shape. Build never emits an
active Codex MCP server entry. Equivalent target-native materialization applies
to other adapters. Hand-concatenating unescaped deployment values is forbidden.

An adapter that builds one concrete mode MUST require link to select exactly
that mode. The initial Codex, Copilot, and Cursor adapters materialize `manual`
for multimodal skills; they reject a deployment that claims to select another
mode. The neutral adapter carries every rendering; link records which of those
renderings deployment selected even when the artifact imports no tools. A future
host projection may install selected renderings, but it MUST NOT validate one
mode's tools while installing another mode's instructions or synthesize new
instructions during link.

A2A Agent Cards are linked from the neutral artifact. For a multimodal agent,
the deployment MUST select `a2a`; only exposed capabilities whose concrete skill
is unimodal or defines an `a2a` variant are advertised.

The same source and unlinked target linked for staging and production MUST retain
identical source and unlinked-target digests. Only the linked artifact and
deployment provenance may differ.

Linked output preserves the input integrity index as historical
`unlinked-build.json`; it MUST NOT leave that index named as though it described
the mutated linked tree. Link provenance records a digest for each linked agent
directory from outside that directory, avoiding a self-digest cycle.

Link may create an absent output directory or populate an existing empty one. It
MUST reject an existing non-empty output unless the directory root contains a
valid schema version 1 `.typeference/link-provenance.json` written by a completed
TypeFerence link. A previously linked output may be recursively replaced; an
unowned directory, file, filesystem root, or path that contains or is contained
by the unlinked input MUST fail before output deletion or writing. The provenance
file is an ownership marker for safe replacement, not a cryptographic trust
claim.

## ARD and callable publication

ARD publication consumes build and optional link artifacts. Source-package and
unlinked-target entries are deterministic. A2A Agent Cards, MCP server cards, and
other callable publications require an explicitly linked endpoint/provider;
TypeFerence MUST NOT invent an address from publisher identity.

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
Stable string ordering is lexicographic by UTF-8 bytes. Resource IDs, mode names,
slot names, field names, and metadata keys use restricted ASCII grammars.

Canonical JSON preserves authored schema member order and decimal lexemes
verbatim, uses defined artifact member order, two-space indentation for files,
compact form for embedded schemas, and deterministic escaping. Map-like objects
sort keys.
Repeated restore, build, pack, and link operations over identical respective
inputs MUST be byte-identical on every platform.

## Diff and security

`typeference diff` compares relative paths and normalized content. Exit code `0`
means identical, `1` changed, and `2` validation/execution failure.

References MUST resolve beneath an authorized package root. Package extraction
MUST prevent path traversal and overwrite outside its materialization directory.
Logs MUST avoid secrets and MCP stdio stdout. Tool annotations and generated
instructions are descriptive, not authorization; hosts remain responsible for
access control and user approval.
