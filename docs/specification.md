# TypeFerence Draft Specification

Status: experimental reference draft, July 2026. The manifest `schemaVersion` is 1; this document is not a claim of ecosystem-standard or production-stable status.

## Scope and non-goals

TypeFerence defines structural composition and deterministic compilation of agent instructions, skill contracts, and context references. It does not define an inference runtime, guarantee equivalent behavior across models or hosts, establish publisher trust, or provide resource discovery.

Agentic Resource Discovery (ARD) can advertise compiled TypeFerence outputs. ARD identifies and locates artifacts; TypeFerence produces target-specific artifacts before publication. Invocation remains the responsibility of MCP, A2A, OpenAPI, or a host-native mechanism.

## Resource identity

A source tree contains YAML documents with `schemaVersion: 1`, a `kind`, and an `id`. IDs use `namespace/name@semantic-version`. Supported kinds are `agent`, `interface`, and `skill`.

## Root object

`system/object@1.0.0` is the universal abstract root. It MUST be behavior-free: no parent, interfaces, skills, slots, norms, context files, or instructions. A non-system enterprise agent MUST directly extend it. TypeFerence owns root semantics; each organization owns its behavioral base.

## Agents

An agent MAY extend exactly one agent and MAY implement multiple interfaces. Except for `system/object`, every agent MUST have a parent. A lineage may be arbitrarily deep but MUST NOT contain a cycle.

Resolution proceeds from the root toward the concrete agent:

1. Scalars use the nearest non-empty derived value.
2. Slots are keyed; a derived value replaces the same key.
3. Norms and context paths append and deduplicate in first-seen order.
4. Skills are keyed by contract ID.
5. Implemented interfaces accumulate through the lineage.

Every resolved contribution records its source resource in provenance.

## Interfaces

Interfaces are contracts only. They MAY require slot names and skill contract IDs. They MUST NOT extend another resource or provide implementation. Resolution fails when a concrete or abstract implementing agent does not satisfy every accumulated requirement.

## Skills and overrides

A skill defines instructions, conditional context references, and JSON input/output schemas. Adding a skill establishes its own ID as the contract ID. An override names both a replacement implementation and the inherited contract it replaces.

An override MUST preserve canonical input and output schemas. It MAY change instructions, description, and conditional context. The derived dispatch name resolves to the nearest implementation while the base agent retains its own namespace.

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

Compilers MUST normalize paths to forward slashes, use LF newlines, serialize keys in ordinal order, omit timestamps, and sort resources by stable ID. Repeated builds from identical source MUST be byte-identical.

The neutral target emits `AGENTS.md`, `bundle.json`, `provenance.json`, and skill folders. Codex, GitHub Copilot, and Cursor adapters emit their native instruction and skill/rule locations. Target-specific outputs MAY add native metadata. An adapter MUST represent each portable resolved field or emit a diagnostic when the target cannot represent it; this requirement does not imply equivalent model behavior.

Behavioral equivalence across hosts is a long-term conformance objective, not a v1 compiler guarantee. Claims of equivalence MUST be supported by target-specific evaluations over declared scenarios and acceptance criteria.

## ARD publication

ARD publication is an optional post-compilation operation. It MUST NOT be modeled as a peer compilation target.

When requested, the reference compiler emits:

1. One canonical source-package catalog entry containing the complete TypeFerence source tree and its SHA-256 digest.
2. One target-bundle entry for every concrete agent and selected compilation target.
3. `derivedFrom` provenance from each target bundle to the canonical source identifier and digest.

The v1 package media types are experimental `application/vnd.typeference.source-package+json` and `application/vnd.typeference.target-bundle+json`. A target bundle contains the exact generated files and names its intended runtime. ARD discovery does not install those files or make one target's format executable by another target. Directly callable services SHOULD instead be published using their native MCP, A2A, OpenAPI, or successor artifact card after deployment.

### Trust metadata compilation

TypeFerence targets the draft AI Catalog Trust Manifest as published at <https://ai-catalog.io/>. Draft evolution MAY require corresponding changes in a future TypeFerence schema version.

A source root MAY contain `typeference.trust.yaml`. The file is part of the canonical source package and its digest, but it is not a typed agent resource and does not participate in inheritance or behavioral resolution. A different trust configuration beneath the source root MAY be selected explicitly. Trust metadata is publication configuration: native target bundles remain usable without ARD.

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

1. Enumerate files recursively and sort their platform paths using ordinal comparison.
2. For each file, append its forward-slash relative path, one NUL byte, its UTF-8 text content with CRLF normalized to LF, and one NUL byte.
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
