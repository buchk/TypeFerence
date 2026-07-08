# ARD Alignment Notes

Status: tracks the **ARD v0.9 draft proposal**; reviewed against open
`ards-project/ard-spec` issues on July 7, 2026. ARD integration in TypeFerence is
optional (`--emit-ard`), excluded from core composition/compilation semantics, and
may require changes in a future TypeFerence schema version as the draft evolves.

TypeFerence should remain a typed authoring and compilation layer. ARD should remain the discovery, registry, publication, and deployed-resource metadata layer. The current ARD issue backlog reinforces that split.

That leaves TypeFerence in distinct territory: it models reusable agent source before runtime artifacts exist. ARD can advertise an `AGENTS.md` bundle, an MCP server, an A2A card, an API, or a workflow; it does not define the high-level language that produced a family of host-native agent instruction artifacts.

## Keep Out of TypeFerence Core

The following ARD proposals are adjacent to TypeFerence output, but should not become TypeFerence source semantics:

| ARD issue | Topic | TypeFerence stance |
| --- | --- | --- |
| [#55](https://github.com/ards-project/ard-spec/issues/55), [#45](https://github.com/ards-project/ard-spec/issues/45) | Release policy, lifecycle, deprecation, migration windows | TypeFerence resource IDs include versions and compiled entries carry versions. Discovery-time lifecycle policy belongs to ARD catalog metadata or a future ARD extension. |
| [#44](https://github.com/ards-project/ard-spec/issues/44) | Deployment metadata, instances, regions, environments | TypeFerence emits static target artifacts and an MCP dispatch package. It must not model runtime instances, replicas, regions, or environment availability. |
| [#42](https://github.com/ards-project/ard-spec/issues/42), [#21](https://github.com/ards-project/ard-spec/issues/21), [#20](https://github.com/ards-project/ard-spec/issues/20), [#22](https://github.com/ards-project/ard-spec/issues/22) | Dependencies, auth hints, access, monetization, filter dimensions | TypeFerence can reference local context and preserve skill contracts. It should not define discovery-time credential feasibility, commercial terms, external dependency manifests, or registry filter semantics. |
| [#43](https://github.com/ards-project/ard-spec/issues/43) | Install-time safety envelope | TypeFerence target bundles are installable artifacts, but consent, revocation, smoke checks, kill switches, and scope grants belong to an install manifest or host installer. |
| [#53](https://github.com/ards-project/ard-spec/issues/53) | Registry federation, mutual trust, cross-registry provenance | TypeFerence provenance links compiled bundles back to canonical TypeFerence source. It should not define registry federation handshakes, trust state machines, or canonical registry selection. |
| [#47](https://github.com/ards-project/ard-spec/issues/47), [#24](https://github.com/ards-project/ard-spec/issues/24) | DID methods, domainless publishers, relay-addressed resources | TypeFerence may validate declared identity syntax for emitted catalog entries. DID method resolution, DNS/relay naming semantics, and publisher authority rules belong to ARD and identity ecosystems. |
| [#41](https://github.com/ards-project/ard-spec/issues/41), [#52](https://github.com/ards-project/ard-spec/issues/52) | Attestation types and trust/compliance caveats | TypeFerence may carry publisher-supplied trust manifest metadata. It must not interpret attestations as compliance, safety, SLSA, or runtime-governance verdicts. |
| [#37](https://github.com/ards-project/ard-spec/issues/37), [#27](https://github.com/ards-project/ard-spec/issues/27) | Recognized media types for skills and bundles | TypeFerence package media types are experimental. Standard ARD-recognized media types should be adopted when ARD or the relevant artifact spec settles them. |
| [#40](https://github.com/ards-project/ard-spec/issues/40), [#34](https://github.com/ards-project/ard-spec/issues/34), [#23](https://github.com/ards-project/ard-spec/issues/23), [#19](https://github.com/ards-project/ard-spec/issues/19), [#18](https://github.com/ards-project/ard-spec/issues/18) | Registry implementation, API conformance, AI Catalog relationship, governance | TypeFerence should consume stable ARD/AI Catalog behavior and keep its own compiler spec scoped to source resources, resolution, target artifacts, and optional publication. |

Reference publisher issues such as [#51](https://github.com/ards-project/ard-spec/issues/51), [#14](https://github.com/ards-project/ard-spec/issues/14), [#13](https://github.com/ards-project/ard-spec/issues/13), [#11](https://github.com/ards-project/ard-spec/issues/11), and [#10](https://github.com/ards-project/ard-spec/issues/10) are useful examples of deployed catalogs. They do not change TypeFerence's core boundary unless they expose a recurring packaging need for compiled static agent bundles.

## Positive Integration Points

TypeFerence should integrate with ARD only at clear publication boundaries:

1. Emit a canonical TypeFerence source-package catalog entry for audit and reproducible compilation.
2. Emit separately versioned target-bundle entries for concrete compiled artifacts.
3. Preserve `derivedFrom` provenance from each target bundle to the canonical source package.
4. Preserve publisher-supplied AI Catalog Trust Manifest fields without dereferencing or judging them.
5. Adopt ARD-standard media types, lifecycle fields, dependency fields, install envelopes, and federation metadata only as catalog-entry metadata when those proposals stabilize.

This lets TypeFerence avoid duplicating ARD while still producing artifacts that ARD can advertise.
