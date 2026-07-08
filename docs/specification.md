# TypeFerence Draft Specification

Status: experimental reference draft, July 2026. Typed resources use `schemaVersion: 3`; this document is not a claim of ecosystem-standard or production-stable status.

## Scope and non-goals

TypeFerence defines structural composition and deterministic compilation of agent instructions, capability contracts, skill implementations, and context references. It does not define an inference runtime, guarantee equivalent behavior across models or hosts, establish publisher trust, or provide resource discovery.

Agentic Resource Discovery (ARD) can advertise compiled TypeFerence outputs. ARD identifies and locates artifacts; TypeFerence produces target-specific artifacts before publication. Invocation remains the responsibility of MCP, A2A, OpenAPI, or a host-native mechanism.

TypeFerence does not define ARD registry lifecycle, release policy, deprecation, deployment instance metadata, dependency manifests, auth feasibility hints, access or monetization policy, install-time consent envelopes, registry federation, DID resolution rules, relay addressing, search filters, registry APIs, or ARD governance. If ARD standardizes any of those fields, TypeFerence MAY preserve them as publication metadata, but they MUST NOT participate in TypeFerence embedding, composition, skill dispatch, or target compilation semantics.

## Resource identity

A source tree contains YAML documents with `schemaVersion: 3`, a `kind`, and an `id`. IDs use `namespace/name@semantic-version`. Supported kinds are `agent`, `profile`, `interface`, `capability`, and `skill`.

## Agents and profiles

An agent is an identity-bearing unit that MAY produce target artifacts. A profile is a reusable composition unit for organizational, domain, or team defaults. Agents MAY embed profiles or agents. Profiles MAY embed profiles but MUST NOT embed agents. Embedding promotes the embedded resources' slots, norms, contexts, and capability bindings into the embedding resource. An embedding graph MUST NOT contain a cycle. No universal root is required.

Profiles participate in composition and validation but do not produce target bundles. This lets users start with `kind: agent` while platform teams define reusable profiles underneath.

Resolution proceeds from embedded resources toward the embedding resource:

1. Display name and description belong to the declaring resource and are not promoted.
2. Slots are promoted by name. The shallowest declaration wins; conflicting declarations at the same depth are ambiguous unless the embedding resource declares that slot locally.
3. Norms and context paths append in embedding order and deduplicate in first-seen order.
4. Capability bindings are promoted by capability ID. The shallowest implementation wins; conflicting implementations at the same depth are ambiguous unless the embedding resource binds that capability locally.
5. Interfaces are computed structurally from the final promoted member set.

Every resolved contribution records its source resource in provenance.

## Interfaces

Interfaces are contracts only. They MAY require slot names and capability IDs, and MAY embed other interfaces. They MUST NOT provide implementation. Every agent whose resolved slots and capability bindings contain all requirements satisfies the interface implicitly; agents do not declare `implements`. Interface embedding MUST NOT contain a cycle.

## Capabilities and skill implementations

A capability defines a stable semantic method slot and its public JSON input/output schemas. It has no instructions and no runtime context.

A skill defines instructions, conditional context references, and JSON input/output schemas. Every skill MUST declare `binds: <capability-id>`. A skill implementation MUST preserve the bound capability's canonical input and output schemas. It MAY change instructions, description, and conditional context.

An agent's or profile's `skills` list binds skill implementations into that resource's resolved method set. If a binding omits `capability`, the capability is inferred from the referenced skill's `binds` field. If it names `capability`, that value MUST match the referenced skill's `binds` field. The outer dispatch name resolves the capability to the nearest compatible skill implementation while the embedded resource retains its own namespace.

## Dispatch

Concrete skills are exposed as `agent-name.skill-name`. Tool names are unique within the TypeFerence MCP server. A call validates required and unknown top-level properties, then returns an invocation package:

```json
{
  "agentId": "helio/payments-repo-agent@1.0.0",
  "skillId": "helio/skills/payments-repository-status@1.0.0",
  "dispatchName": "payments-repo-agent.repository-status",
  "arguments": { "focus": "release" },
  "instructions": "...",
  "contextReferences": ["context/payments-service.md"],
  "targetHints": { "codex": ".agents/skills" },
  "provenance": []
}
```

Hosts execute the package. The v1 server does not call an LLM.

## Deterministic compilation

Compilers MUST normalize paths to forward slashes, use LF newlines, omit timestamps, and sort resources by stable ID. Repeated builds from identical source MUST be byte-identical, and independent conforming implementations MUST produce byte-identical artifacts from identical source (see the conformance suite under `conformance/`).

### Canonical text and ordering

- All source and artifact files are UTF-8 text. Implementations MUST ignore a leading byte-order mark when reading and MUST write artifacts as UTF-8 without a byte-order mark. Behavior on invalid UTF-8 input is unspecified; implementations MAY reject it.
- Wherever this specification requires sorting or deduplicating strings, the ordering is lexicographic by Unicode code point (equivalently: by UTF-8 byte sequence). Implementations MUST NOT use UTF-16 code-unit order, culture-aware collation, or case folding. To keep the divergent encodings' orderings observably identical, canonical key spaces are restricted to ASCII: resource IDs (already ASCII by grammar), slot names, and trust metadata keys MUST match `^[A-Za-z0-9][A-Za-z0-9._-]*$`, and trust identity URIs MUST be ASCII (internationalized authorities MUST be pre-encoded as punycode).
- Emitted artifact files always end with LF-terminated content exactly as specified per artifact; no implementation-chosen trailing whitespace is permitted.

### Canonical JSON serialization

JSON artifacts (`bundle.json`, `provenance.json`, `ai-catalog.json`) are serialized as follows:

- **Member order.** Objects serialize their members in the canonical order defined per artifact shape (for `bundle.json`: `id`, `displayName`, `description`, `emit`, `embeds`, `satisfies`, `slots`, `workingNorms`, `contextFiles`, `skills`, `provenance`; skills: `dispatchName`, `capabilityId`, `implementationId`, `description`, `instructions`, `inputSchema`, `outputSchema`, `contextFiles`, `provenance`; provenance entries: `field`, `source`). Map-like objects (slots, trust manifests, trust metadata) serialize keys in canonical string order as defined above. Member order is not alphabetical unless stated.
- **Layout.** Indented artifacts use two-space indentation, `": "` after keys, one member or element per line, `[]` and `{}` for empty collections, and LF line endings, followed by one trailing LF at end of file. Canonical embedded JSON (schema strings) uses the compact layout with no whitespace.
- **String escaping.** ASCII characters 0x20–0x7E are emitted literally except `"` `&` `'` `+` `<` `>` `` ` `` and `\`. Backspace, tab, line feed, form feed, and carriage return use the two-character escapes `\b` `\t` `\n` `\f` `\r`; backslash is `\\`. Every other character — remaining control characters and all code points above 0x7E — is escaped as `\uXXXX` with uppercase hexadecimal digits, using UTF-16 surrogate pairs for supplementary-plane code points.
- **Numbers.** JSON number tokens carried through canonicalization (for example inside capability schemas) are preserved byte-for-byte as authored; implementations MUST NOT reformat `1.0` as `1` or normalize exponent notation.
- **Schema canonicalization.** A JSON schema's canonical form is its compact serialization under the rules above, preserving authored member order, duplicate keys, and number tokens. Schema equality (capability contract preservation) is byte equality of canonical forms.

The neutral target emits `AGENTS.md`, `bundle.json`, `provenance.json`, and skill folders. Codex, GitHub Copilot, and Cursor adapters emit their native instruction and skill/rule locations. Target-specific outputs MAY add native metadata. An adapter MUST represent each portable resolved field or emit a diagnostic when the target cannot represent it; this requirement does not imply equivalent model behavior.

Behavioral equivalence across hosts is a long-term conformance objective, not a v1 compiler guarantee. Claims of equivalence MUST be supported by target-specific evaluations over declared scenarios and acceptance criteria.

## ARD publication

ARD publication is an optional post-compilation operation. It MUST NOT be modeled as a peer compilation target.

When requested, the reference compiler emits:

1. One canonical source-package catalog entry containing the complete TypeFerence source tree and its SHA-256 digest.
2. One target-bundle entry for every concrete agent and selected compilation target.
3. `derivedFrom` provenance from each target bundle to the canonical source identifier and digest.

The v1 package media types are experimental `application/vnd.typeference.source-package+json` and `application/vnd.typeference.target-bundle+json`. A target bundle contains the exact generated files and names its intended runtime. ARD discovery does not install those files or make one target's format executable by another target. Directly callable services SHOULD instead be published using their native MCP, A2A, OpenAPI, or successor artifact card after deployment.

TypeFerence-generated catalog entries intentionally omit ARD-owned lifecycle, deployment, dependency, install-safety, federation, and registry-search metadata unless supplied as external publication metadata. TypeFerence source versions describe authoring resources; they do not imply discovery-time availability, migration windows, deprecation state, supported regions, credential requirements, commercial terms, or registry federation consent.

### Trust metadata compilation

TypeFerence targets the draft AI Catalog Trust Manifest as published at <https://ai-catalog.io/>. Draft evolution MAY require corresponding changes in a future TypeFerence schema version.

A source root MAY contain `typeference.trust.yaml`. The file is part of the canonical source package and its digest, but it is not a typed agent resource and does not participate in embedding or behavioral resolution. A different trust configuration beneath the source root MAY be selected explicitly. Trust metadata is publication configuration: native target bundles remain usable without ARD.

The trust configuration has `schemaVersion: 1` and MAY contain `source` and `bundles` profiles. At least one profile is required. A source profile requires a literal `identity`. A bundles profile requires an `identityTemplate` containing both `{agent}` and `{target}`; `{publisher}` and `{version}` are also supported. Each profile MAY contain:

- `identityType`
- an AI Catalog `trustSchema`
- AI Catalog `attestations`
- additional AI Catalog `provenance` links
- arbitrary JSON-compatible `metadata`
- `signatureIntent` containing an algorithm, key reference, and optional required flag

TypeFerence MUST preserve its generated `derivedFrom` link as the first provenance link for every target bundle. Configured links follow it in source order. It MUST add a deterministic target artifact digest to `com.github.buchk.typeference.artifactDigest` in Trust Manifest metadata. The digest is covered when the Trust Manifest is externally signed.

Trust metadata is declarative. TypeFerence MUST NOT dereference identity, attestation, policy, provenance, or key URIs; issue compliance claims; infer a SLSA level; claim that runtime governance executed; or treat referenced TRACE-style runtime evidence as deployment state. A referenced attestation asserts only that the publisher supplied that reference.

Identity and URI syntax, known publisher-domain bindings, attestation shape, digest encoding, template placeholders, and metadata keys MUST be validated locally. Identity schemes remain open, but known `did`, `spiffe`, and `https` scheme/type contradictions MUST be rejected. Digests MUST be lowercase SHA-256, SHA-384, or SHA-512 values in `algorithm:hex` form.

### Artifact digest algorithm

`typeference-directory-v1` hashes a text artifact directory as follows:

1. Enumerate files recursively and sort their forward-slash relative paths in canonical string order (Unicode code point order; see Deterministic compilation). Sorting platform-native paths is non-conforming: platform separators (`\` vs `/`) order differently against other characters, which makes the digest platform-dependent.
2. For each file, append its forward-slash relative path, one NUL byte, its UTF-8 text content with any leading byte-order mark removed and CRLF normalized to LF, and one NUL byte.
3. SHA-256 hash the UTF-8 encoding of the resulting sequence and encode it as lowercase hexadecimal with a `sha256:` prefix.

The v1 package formats contain text files only. A future binary package format MUST define a different digest scheme rather than silently changing this algorithm.

### Trust Manifest signatures

The AI Catalog `signature` member is reserved for a compact detached JWS over the RFC 8785 JCS-canonicalized Trust Manifest after removing `signature`. TypeFerence MUST NOT place placeholders in this field.

TypeFerence MAY import externally created detached JWS strings from a JSON object keyed by generated catalog identifier. It validates compact detached form but does not verify cryptographic validity or resolve keys. Unknown identifiers MUST be rejected. When `signatureIntent.required` is true, publication MUST fail unless the entry has an imported signature. An explicit unsigned-staging option MAY bypass this check solely to emit the payload for an external signer. Signing intent is emitted under `com.github.buchk.typeference.signatureIntent` with invariant `status: external`; whether signing is fulfilled is represented solely by the standard `signature` member. This metadata MUST NOT change when a signature is injected, because it is part of the signed payload.

The signature map MUST reside outside the source root. This prevents a cycle in which adding a signature changes the source-package digest embedded in the content being signed. Repeated builds from identical source, trust configuration, and signature map MUST be byte-identical.

## Diff contract

`typeference diff` compiles to temporary storage and compares relative paths and content. Exit code `0` means identical, `1` means changed, and `2` means validation or execution failed.

## Security

All source references MUST resolve beneath the source root and MUST exist. MCP stdio logging MUST avoid stdout. Tool annotations and generated instructions are descriptive, not authorization. Hosts remain responsible for access control and user approval.
