# GitHub Issue Draft: ARD Authoring Layer Question

## Title

Design: pre-publication authoring/compilation layer for agent resources -- in scope for ARD?

## Body

### Summary

I would like to sanity-check whether the ecosystem around ARD needs guidance for a **pre-publication authoring/compilation layer**.

Before a publisher advertises something through ARD, they need a coherent way to author, compose, and build the actual resources they are publishing: instructions, skills, context references, target formats, provenance, trust metadata, and so on. ARD is focused on discovery and verification of published artifacts, but the "how do I produce something good to publish?" step seems to be left to each project.

My question is: **does ARD, or the broader ecosystem around ARD, need guidance for this layer, or is it intentionally outside ARD's scope?**

The thing I am trying to avoid is a world where discovery becomes standardized, but the artifacts being discovered are still hand-authored, host-specific instruction blobs with unclear provenance, inconsistent build process, and no reproducible trust story.

I am not asking ARD to adopt a particular compiler or source language. I am asking whether the ecosystem needs a common way to talk about pre-compiled or authored agent resources, their target runtime, and the provenance/trust metadata connecting those resources to the source and build process that produced them.

A concrete example is **TypeFerence**, a small reference experiment I am working on. It treats agent definitions as typed source and compiles them into host-native artifacts such as `AGENTS.md`, Copilot instructions, Cursor rules, neutral bundles, and optional ARD catalog entries. It is not a runtime, registry, identity system, deployment system, or discovery protocol.

The analogy is:

- runtime system prompts are like machine code
- `AGENTS.md` and similar host-native instruction files are like assembly language
- TypeFerence is a higher-level language that compiles into those native forms

This issue is not asking ARD to standardize TypeFerence itself, and it is not asking ARD to become a source-code registry or build system. If the answer is "this is out of scope; use custom media types and provenance fields," that is useful feedback too.

### Non-goals

This issue is not proposing that ARD:

- standardize TypeFerence or any particular authoring language
- store source code
- define deployment, install, consent, runtime, lifecycle, federation, or identity semantics
- require clients to compile source during discovery or invocation

The question is only whether there should be guidance for pre-publication artifact production, target identity, provenance, digests, and trust-manifest signing inputs.

### Why this seems adjacent to ARD

ARD already does the right thing by staying artifact-agnostic: it can advertise MCP servers, A2A cards, APIs, workflows, skill artifacts, catalogs, and other resources without owning each artifact's internal format.

Pre-compiled or authored agent resources may fit that model:

1. A publisher may want to advertise a precompiled bundle for a concrete host target.
2. Each compiled bundle should point back to source provenance without requiring ARD to store or standardize that source.
3. Discovery clients should not assume that a static bundle is directly callable.
4. Installation, consent, deployment, identity resolution, lifecycle, and federation should remain separate ARD / host / installer concerns.

The practical concern is that without some shared guidance, different projects may invent incompatible ways to describe "this agent resource was built from that source, for this target, with this digest, under this publisher identity."

### Example catalog shape

If this layer is worth recognizing, a catalog entry for a pre-compiled static agent bundle might look something like this. This is meant as an example, not a proposed normative format:

```json
{
  "identifier": "urn:air:example.com:typeference:codex:payments-repo-agent",
  "displayName": "Payments Repo Agent (codex)",
  "type": "application/vnd.typeference.target-bundle+json",
  "description": "Precompiled Codex artifact bundle.",
  "version": "1.0.0",
  "capabilities": [
    "payments-repo-agent.repository-status"
  ],
  "data": {
    "schemaVersion": 1,
    "target": "codex",
    "agentId": "example/payments-repo-agent@1.0.0",
    "files": []
  },
  "metadata": {
    "generatedBy": "TypeFerence",
    "sourceUri": "https://github.com/example/agents/tree/0123456789abcdef",
    "sourceDigest": "sha256:...",
    "target": "codex"
  },
  "trustManifest": {
    "identity": "https://example.com",
    "identityType": "https",
    "provenance": [
      {
        "relation": "derivedFrom",
        "sourceId": "https://github.com/example/agents/tree/0123456789abcdef",
        "sourceDigest": "sha256:..."
      }
    ]
  }
}
```

The media types above are experimental placeholders from the TypeFerence prototype. They are included only to make the shape concrete, not as a request that ARD bless these exact strings.

### Relationship to existing ARD issues

The closest existing threads seem to be #27 and #37 on recognized artifact/media types. This also touches the `trustManifest` conversation in #41, #52, and #40, especially around what evidence an attestation or signed manifest does and does not prove.

I am explicitly trying not to overlap with lifecycle and release policy (#45, #55), install-time safety envelopes (#43), deployment metadata (#44), dependencies/auth/access metadata (#42, #21, #22), federation (#53), or identity/relay naming (#47, #24).

The pre-publication authoring/compiler layer should not define those things. A build tool can emit or preserve ARD metadata once ARD standardizes it, but those fields should not participate in the tool's own source-language semantics unless that language chooses to model them separately.

### Trust Manifest implications

This may also have implications for the AI Catalog / ARD `trustManifest` format.

A build tool or compiler can be a natural producer of trust-manifest metadata if the target shape is sufficiently stable. The important separation is that the tool produces deterministic signing input, while a publisher-controlled key signs the claim. For example, a tool can deterministically emit:

- `provenance` entries linking a compiled bundle to the source URI, commit, package reference, or digest it was built from
- an artifact digest for the compiled bundle
- publisher-supplied `identity`, `identityType`, `trustSchema`, and `attestations`
- signing input for an external signer
- a detached signature once an external signing step has completed

The resulting verification chain can stay language-agnostic:

1. Source exists at a URI, commit, package reference, and/or digest.
2. A build tool produces a deterministic target resource or bundle.
3. The tool computes the target artifact digest and emits a Trust Manifest payload saying the artifact was derived from that source.
4. A publisher-controlled signing key signs the payload.
5. An ARD registry verifies the signature, signer identity, source provenance fields, and artifact digest without understanding the tool's source language or composition semantics.

That fits the direction of #41 and #52: attestations and filters should remain carefully scoped evidence, not broad compliance or safety verdicts. It also relates to #40's practical conformance question about what counts as an acceptable `trustManifest` for a publisher.

The pre-publication build angle adds one design pressure: if ARD / AI Catalog standardizes canonicalization, digest scope, provenance relations, signer identity binding, and detached-signature placement, then build tools can target that format directly and produce reproducible trust metadata for published agent resources. If those pieces remain underspecified, each tool will likely invent slightly different metadata keys for source provenance, artifact integrity, and signing payloads.

### Questions

1. Is a pre-publication authoring/compilation layer even a problem worth solving, or will future models and tools make ad hoc prompt/resource management sufficient?
2. Assuming it is worth addressing, should ARD or the broader ecosystem provide any guidance for this layer, or is it intentionally out of scope?
3. If it is in scope, what level of guidance would be useful?
   - Recognizing pre-compiled or authored agent resources as catalog resources.
   - Documenting `derivedFrom` provenance from published artifacts back to source repositories, commits, packages, or digests.
   - Defining enough Trust Manifest canonicalization, digest, provenance, signer identity, and signature semantics for build tools to target it reproducibly.
   - Distinguishing static host instruction bundles from directly callable resources such as MCP servers, A2A agents, and OpenAPI endpoints.
4. If it is out of scope, is this a real coordination problem that belongs in a different venue than ARD, or is per-tool fragmentation here an acceptable or expected outcome?

### Example implementation

[TypeFerence](https://github.com/buchk/TypeFerence) currently compiles a small typed source model into:

- neutral `AGENTS.md` bundles
- Codex `AGENTS.md` plus skill folders and MCP config
- GitHub Copilot instructions / custom agent profile files
- Cursor rules
- optional ARD `ai-catalog.json` publication with compiled target-bundle entries and source provenance

The important boundary I am trying to preserve is:

- an authoring/compiler tool owns structural composition, deterministic compilation, artifact diffs, and source-to-bundle provenance.
- ARD owns discovery, registry behavior, publication metadata, trust signaling, lifecycle, federation, deployment metadata, and how consumers find resources.

I am looking for feedback on whether that boundary seems complementary, whether ARD or the surrounding ecosystem needs any standard hooks at this layer, or whether I am thinking about the problem at the wrong level.

### Personal note

I am at the edge of my depth here. I built what feels like a useful reference implementation because the problem of maintaining coherent behavior across many agents feels real to me, but I genuinely do not know how this looks from the perspective of standards maintainers, large platform teams, or people with deeper experience in this space.

That is why I am asking. I want the feedback, even if the answer is "this layer is not needed," "this belongs entirely outside ARD," or "you are thinking about the problem at the wrong level."
