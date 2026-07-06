# TypeFerence

**Define organizational agents once. Validate their composition. Compile them into the native shapes your teams already use.**

TypeFerence is an experimental reference implementation of a typed definition and compilation layer for AI agents. It replaces sprawling, duplicated instruction files with Go-like composition: agent embedding, structurally satisfied interfaces, versioned skill contracts, deterministic compilation, provenance, and artifact diffing.

Read the [whitepaper](docs/whitepaper.md), the [rendered PDF](output/pdf/typeference-whitepaper.pdf), or the [draft v2 specification](docs/specification.md).

```text
helio/enterprise-agent ──embedded by──> helio/person-agent ──embedded by──> helio/executive-assistant
                       └─embedded by──> helio/repo-agent   ──embedded by──> helio/payments-repo-agent
```

There is no universal root and no nominal `implements` declaration. Agents embed reusable agents, promoted behavior is checked for ambiguity, and interfaces are discovered from the resulting slot and skill set.

## Where it fits

```text
TypeFerence source
    -> native agent artifacts
    -> optional ARD publication and discovery
    -> MCP, A2A, OpenAPI, or host-native invocation
    -> Codex, Copilot, Cursor, Yoke, or another runtime
```

[Agentic Resource Discovery](https://agenticresourcediscovery.org/) helps clients find and verify deployed capabilities. TypeFerence addresses the earlier authoring problem: producing compatible native artifacts from one governed definition. Discovery portability does not itself provide definition portability.

The long-term objective is behavioral equivalence: preserving declared organizational intent across supported hosts closely enough to be measured and governed. V2 provides the common typed source, deterministic adapters, and provenance needed to test that objective; it does not claim that different models or runtimes already behave identically.

The v2 source shape is deliberately small:

```yaml
schemaVersion: 2
kind: agent
id: helio/payments-repo-agent@1.0.0
embeds:
  - helio/repo-agent@1.0.0
skills:
  - ref: helio/skills/payments-repository-status@1.0.0
    contract: helio/skills/repository-status@1.0.0
```

Use `emit: false` for reusable agents that should participate in composition without producing their own target bundle.

## Quick start

Requires the .NET 10 SDK.

```powershell
dotnet restore TypeFerence.slnx
dotnet build TypeFerence.slnx

$tf = "src/TypeFerence.Cli/bin/Debug/net10.0/typeference.dll"
dotnet $tf validate examples/helio
dotnet $tf build examples/helio --target all --out dist
dotnet $tf build examples/helio --target all --out dist --emit-ard --publisher-domain helio.example
dotnet $tf inspect helio/payments-repo-agent@1.0.0 --source examples/helio
dotnet $tf diff examples/helio --against dist
dotnet $tf serve dist/neutral
```

The generated MCP server exposes tools such as:

- `executive-assistant.prepare-brief`
- `payments-repo-agent.repository-status`

Calls return a deterministic invocation package for the host agent. TypeFerence does not embed an LLM provider.

`--emit-ard` publishes one canonical TypeFerence source-package entry and one precompiled bundle entry for each concrete agent and selected target. Compiled entries carry `derivedFrom` provenance back to the canonical source digest. ARD is an envelope: a target-aware installer must understand static Codex, Copilot, or Cursor bundles, while callable MCP or A2A resources require their own deployed endpoint and native card.

### Trust manifests

An optional `typeference.trust.yaml` at the source root enriches the draft AI Catalog `trustManifest` for the source package and compiled bundles. It can declare DID, SPIFFE, or HTTPS identity; trust schemas; attestation and provenance references; policy or enterprise verification metadata; and an intent to sign. TypeFerence validates and publishes these declarations without resolving remote documents or asserting that an external authority has verified them.

```yaml
schemaVersion: 1
source:
  identity: did:web:helio.example:typeference:source:helio
  identityType: did
  attestations:
    - type: https://slsa.dev/provenance/v1
      uri: https://trust.helio.example/provenance/source.intoto.jsonl
      digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
bundles:
  identityTemplate: spiffe://helio.example/typeference/{target}/{agent}
  identityType: spiffe
  metadata:
    com.helio.governance:
      policyDigest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      runtimeEvidenceProfile: tag:agentrust.io,2026:trace-v0.1
  signatureIntent:
    algorithm: ES256
    keyRef: did:web:helio.example#catalog-signing
```

TypeFerence does not hold signing keys. An external signer can produce detached JWS values over the unsigned, JCS-canonicalized trust manifests and provide them through `--trust-signatures signatures.json`. Use `--allow-unsigned-trust` only to emit signing input when `signatureIntent.required` is true; normal publication fails closed without those signatures. The signature map must be outside the source root so adding signatures cannot change the source digest they sign. Identical source, trust config, and signature map inputs produce byte-identical output.

## Repository map

- `src/` - compiler, target adapters, CLI, and MCP runtime.
- `examples/helio/` - fictional cross-domain organization.
- `docs/specification.md` - normative v2 behavior.
- `docs/whitepaper.md` and `output/pdf/typeference-whitepaper.pdf` - design paper.
- `tests/` - type-system, target, determinism, and MCP integration tests.

## Design boundaries

- Agents may embed multiple agents; local slots and skills resolve promoted-name ambiguity.
- Interfaces may embed interfaces and are satisfied structurally, without declarations on agents.
- Skill contracts are explicit and versioned; a local implementation may name the contract it fulfills.
- Context is referenced and loaded only with the skill that needs it.
- Target adapters emit platform-native shapes while retaining the portable fields each target supports.
- Build output is deterministic and carries provenance.
- No deployment state, hosted runtime, or model credentials in v2.
- Structural validation does not guarantee identical LLM behavior across models or hosts.
- ARD publication wraps selected target outputs; it is not itself a compilation target or execution runtime.

The current adapters are reference implementations against fast-moving platform formats. Review generated artifacts before production use.

TypeFerence is licensed under Apache-2.0. Helio Works is fictional.
