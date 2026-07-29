# TypeFerence Draft Specification

Status: experimental reference draft, July 2026. Typed resources use
`schemaVersion: 4`; project manifests use `schemaVersion: 2`. Version 4 is a
deliberate closure of the source language. Unsupported fields and schema versions
are errors rather than extension points.

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

A source root MAY contain `typeference.yaml`:

```yaml
schemaVersion: 2
name: marathon/payments-agents
version: 1.0.0
publisher: marathon.example
dependencies:
  marathon/enterprise-foundations: 3.1.0
```

`name`, `version`, and every dependency version are required when the manifest is
present. Versions are exact semantic versions in version 4; range solving is not
defined. Dependency names use the resource namespace grammar. `publisher` is
optional stable publication identity. Unknown fields, deployment fields, feed
addresses, and credentials are errors.

## Resource identity and kinds

A compilation unit contains UTF-8 YAML or frontmatter-plus-body documents with
`schemaVersion: 4`, a `kind`, and an `id`. IDs use
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

## Composition

Agents MAY embed profiles or agents. Profiles MAY embed profiles but MUST NOT
embed agents. An embedding graph MUST NOT contain a cycle.

Resolution proceeds from embedded resources toward the embedding resource:

1. Display name and description belong to their declaring resource.
2. Slots promote by name. The shallowest declaration wins; different
   declarations at the same depth are ambiguous unless declared locally.
3. Norms and held context IDs append in embedding order and deduplicate in
   first-seen order.
4. Capability bindings promote by capability ID. The shallowest compatible
   implementation wins; different implementations at the same depth are
   ambiguous unless bound locally.
5. Allow-lists intersect. A disjoint intersection is an explicit empty set, not
   an unrestricted set.
6. Every contribution records source-resource provenance.

Profiles may retain abstract required capability bindings. Agents are concrete
and MUST fulfill every promoted requirement.

## Interfaces, capabilities, skills, and bindings

Interfaces MAY require slot names and capability IDs and MAY embed interfaces.
They provide no implementation. Satisfaction is inferred from the resolved
member set.

A capability defines canonical JSON `inputSchema` and `outputSchema`. A skill
declares `binds: <capability-id>` and MUST preserve those schemas byte-for-byte
after canonicalization. Its instructions provide the model implementation.

Bindings have independent presence and mutability axes:

```yaml
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
binding is an error.

## Invocation modes

A skill declares either `instructions` or `variants`, never both. Base
requirements apply to every mode; variant requirements are additive:

```yaml
kind: skill
requiresTools: [acme/tools/repository@1.0.0]
variants:
  manual:
    instructions: Explain the evidence.
  pipeline:
    instructions: Emit strict JSON.
    requiresTools: [acme/tools/build-signals@1.0.0]
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

```yaml
schemaVersion: 4
kind: contextType
id: acme/context-types/team@1.0.0
fields:
  owner:
    type: string
    required: true
  participants:
    type:
      list: string
    required: true
  governed:
    type: boolean
    default: false
  attributes:
    type:
      map: string
body:
  type: text
  required: false
```

Version 4 supports `string`, `text`, `boolean`, `integer`, `number`, `list<T>`,
`map<T>`, and exact references to named context types. A field may be `required`
and may declare a type-correct scalar or collection `default`. Unknown
constructors and unsupported constraints such as unions, `oneOf`, regular
expressions, arbitrary `$ref`, or open properties are errors.

A context instance declares its type and values:

```yaml
schemaVersion: 4
kind: context
id: acme/context/payments-team@1.0.0
contextType: acme/context-types/team@1.0.0
values:
  owner: payments-platform
  participants: [Ari, Sam]
---
Stable prose carried as the typed body.
```

Values are parsed schema-directed; YAML implicit scalar typing MUST NOT decide
their meaning. Missing required fields, unknown fields, wrong shapes, and invalid
named-type references are errors. Defaults are materialized before hashing and
emission. Resolved artifacts preserve the complete canonical value object and
body, so any semantic context change changes target bytes.

Context types MAY embed other context types to refine them. Field redeclaration
must preserve the original type and may only strengthen optional to required.
A context satisfies its declared type and every embedded base type.

Agents and profiles hold context by resource ID using `context`. Skills require
types using `requiresContextTypes`. `allowedContextTypes` is a closed whitelist;
omitting it means unrestricted and specifying `[]` means no context is allowed.
Allow-lists intersect through composition without treating the empty result as
unrestricted.

`contextFiles` does not exist in version 4. Raw prose is represented honestly as
a context type with a required `text` body. Slot values that carry context MUST
reference context resource IDs, never filesystem paths. All referenced files and
resources MUST be explicit members of the source package and resolve beneath its
root.

JSON Schema MAY be emitted as a target interoperability projection of a native
context type. It is not accepted as the source language and never controls
TypeFerence validation semantics.

## Tools as runtime imports

A `tool` is an extern declaration: a versioned runtime import with canonical
input/output schemas and optional non-secret requirement metadata. It contains no
implementation, command, endpoint, environment-specific scope, or credential
value.

A skill or variant lists exact tool IDs in `requiresTools`. Build verifies that
each declaration exists, is a `tool`, and is structurally valid. It does not
pretend the runtime implementation exists. Link binds each effective required
tool to one compatible provider in the deployment file and fails closed if
missing or incompatible.

The vocabulary is intentionally separate:

- capability: exported semantic contract;
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
`obj/typeference/packages`. `restore --locked` MUST reject any manifest/lock
disagreement and MUST NOT rewrite the lock. `typeference update <package>` is the
explicit operation that rewrites selected lock entries.

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
output path is in a normatively excluded generated directory.

`typeference-resource-set-v1` sorts normalized source-relative paths by UTF-8 byte
order and hashes, for each file, `path`, NUL, normalized content, NUL using
SHA-256. Target provenance records the root source digest, the ordered locked
dependency digests, and compiler/adapter version.

`typeference-directory-v1` remains the target-directory digest: recursively sort
forward-slash paths, then hash `path`, NUL, normalized text, NUL. It applies only
to already-defined artifact directories, not source identity.

## Build targets

Build emits deterministic **unlinked** packages for neutral, Codex, GitHub
Copilot, and Cursor targets. Each adapter MUST represent every portable resolved
field or emit a diagnostic. Host-native active runtime configuration is not a
build artifact.

An unlinked target contains instructions, skills, native rules, bundle metadata,
mode requirements, tool imports, complete compiled context values, provenance,
and a link-requirements manifest. It MUST NOT contain invented endpoints,
commands, unresolved `${TOKEN}` text in active configuration, or environment
defaults.

## Deployment files and link

Deployment is supplied explicitly outside the source package:

```yaml
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
values. Environment entries are references (`fromEnvironment` or a future
typed secret-provider reference), never secret values. URLs MUST be absolute
HTTPS URLs unless a transport-specific local-development option explicitly
allows otherwise.

`typeference link <built-target> --deployment <file> --out <dir>`:

1. verifies the unlinked target digest;
2. selects the artifact modes;
3. validates every effective tool import and endpoint requirement;
4. structurally serializes host-native configuration;
5. records deployment-file and materialized-artifact digests separately.

For Codex, local stdio MCP bindings materialize `.codex/config.toml` using TOML
string escaping; HTTP bindings use the native URL shape. Build never emits an
active Codex MCP server entry. Equivalent target-native materialization applies
to other adapters. Hand-concatenating unescaped deployment values is forbidden.

The same source and unlinked target linked for staging and production MUST retain
identical source and unlinked-target digests. Only the linked artifact and
deployment provenance may differ.

## ARD and callable publication

ARD publication consumes build and optional link artifacts. Source-package and
unlinked-target entries are deterministic. A2A Agent Cards, MCP server cards, and
other callable publications require an explicitly linked endpoint/provider;
TypeFerence MUST NOT invent an address from publisher identity.

Publisher identity may participate in stable URNs and trust identity. Endpoint
addresses do not. Publication remains an explicit edge operation and may perform
network I/O only when requested.

## Trust metadata

A source root MAY contain `typeference.trust.yaml` or select one explicitly. Trust
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

Canonical JSON preserves schema number tokens and authored schema member order,
uses defined artifact member order, two-space indentation for files, compact form
for embedded schemas, and deterministic escaping. Map-like objects sort keys.
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
