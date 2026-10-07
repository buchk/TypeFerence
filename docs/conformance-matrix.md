# Specification evidence matrix

This matrix connects the normative version 8 specification to executable
evidence. It is a review aid, not a second specification: when text and tests
disagree, `docs/specification.md` wins and both the implementation and evidence
must be corrected.

Fixture numbers refer to `conformance/fixtures/`: `001`-`099` succeed with
recorded digests, `101` onward must fail with a diagnostic. Canonicalization and
composition decisions require a golden fixture in the same change; focused Go
tests cover diagnostics, internal invariants, and rendered content.

| Normative area | Golden evidence | Focused implementation evidence |
| --- | --- | --- |
| Manifest, version 8 only | 116 | `internal/compile`: every build test; `internal/lsp`: `TestServerReportsDocumentsOutsideAPackage` |
| Kinds from suffixes; removed kinds | 115 | `internal/resource` decoding through every fixture |
| Removed fields (`sealed`, `a2a`) | 123, 126 | — |
| Descriptions after rendering | 128 | — |
| Frontmatter grammar | 010 | `internal/tferlex`: all tests; `internal/lsp`: `TestServerDiagnostics` |
| Plugins, modes, artifact layout | 002, 005, 008 | `internal/compile`: `TestServersShipWithTheirSkillsPerMode` |
| Plugins as the binding root: embedding profiles and plugins, promotion, shallowest wins, rebinding by slot, required capabilities; packaging not inherited | 009, 012, 020, 021, 121, 122, 124, 160, 162, 164, 165 | `internal/compile`: `TestProvenanceKeepsEveryContributor` |
| Agent extension and roles; one agent file per agent name | 009, 013, 020, 022, 158, 159, 161, 163 | `internal/compile`: `TestEmbeddedPluginBindingsShallowestWins` |
| Additive extension across modes | 008, 113 | `internal/packages`: `TestRestoreMaterializesTransitiveGraphForOfflineBuild` |
| Documents and data; context types | 003, 004, 106, 107, 108, 114 | `internal/compile`: `TestContractChangeFailsEveryInstance` |
| Parameters and field references | 003, 004, 102, 120 | `internal/compile`: `TestAgentInstantiatesTemplates`, `TestOptionalFieldWithoutValueCannotBeReferenced` |
| Instances and instance names; embedded plugins' bindings | 003, 004, 013, 020, 101, 103, 104, 105, 111, 119, 125, 129 | `internal/compile`: `TestAgentInstantiatesTemplates`, `TestEmbeddedPluginBindingsShallowestWins` |
| Rendering held documents: a plugin's, then its agent's | 003, 021, 022 | `internal/compile`: `TestAgentInstantiatesTemplates` |
| Skill files and schemas; portable destinations | 006, 011, 112, 127, 130, 131 | `internal/compile`: `TestSchemasAndFilesShipBesideTheSkill`, `TestSkillDirectoryPaths`; `internal/packages`: `TestBinarySkillFilesPackByteForByte` |
| Servers and `mcp.json` | 005, 109, 110, 118 | `internal/compile`: `TestServersShipWithTheirSkillsPerMode`; `go/conformance`: `TestMarketplaceServerNamesDenoteOneConfiguration` |
| Native components: rules, commands, hooks, LSP servers, agent-scoped servers; enterprise defaults | 014, 021, 137, 138, 139, 140, 141, 142, 143 | `internal/compile`: `TestNativeComponentsRender`; `internal/importer`: `TestImportCarriesCopilotComponents` |
| Output contract | every success fixture | `tools/validate_output.py` in CI's `output-contract` job |
| Agent tool allowlists: `tools: []`, coverage of the plugin's servers per mode | 015, 144, 145, 146, 147, 148 | `internal/compile`: `TestAgentToolAllowlists`; `internal/importer`: `TestImportKeepsAnAgentWithNoTools`, `TestImportNotesAgentToolsThatMissAServer` |
| Plugin metadata; plugin and marketplace carried files; root files covered by `build.json` | 016, 017, 018, 149, 150, 151, 152, 153, 154 | `internal/importer`: `TestImportCarriesPluginMetadataAndFiles`, `TestImportFailsClosedOnPluginManifestAndStrayComponentFiles`, `TestImportRejectsAMarketplaceRepository`; `tools/validate_output.py` root-file digests |
| Marketplace catalog fields: plugin `category` and `tags`, marketplace `description`; absent from `plugin.json` | 019, 155, 156, 157 | `internal/importer`: `TestImportCarriesCatalogFields`; `tools/validate_output.py` marketplace schema |
| Copilot fields | 007 | `internal/compile`: `TestCopilotFieldsRenderInTableOrder`; `internal/importer`: `TestImportCarriesFilesServersAndCopilotFields` |
| Build-wide name uniqueness | 117, 118 | `go/conformance`: `TestCandidateCollisionIsReported` |
| Packages, restore, lockfiles, Git routes | 001 | `internal/packages`: all tests |
| Organization marketplaces and candidates | 001 | `go/conformance`: `TestMarketplaceBumpRewritesOnlyTheOwnersPlugins`, `TestCandidate*` |
| Canonical text; binary-safe digests and diff | 010, 011 | `internal/jsonx`: all tests; `internal/compile`: `TestDiffAndDigestAreExactForBinaryFiles` |
| Import | — | `internal/importer`: all tests |
| Reference output | — | `internal/compile`: `TestHelioReference` |
