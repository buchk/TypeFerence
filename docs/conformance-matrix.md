# Specification evidence matrix

This matrix connects the normative version 6 specification to executable
evidence. It is a review aid, not a second specification: when text and tests
disagree, `docs/specification.md` wins and both the implementation and evidence
must be corrected.

Current conformance fixtures omit the manifest `language` field. Fixtures marked
`legacy-v5` or `legacy-v3` are archival byte regressions and never count as the
only evidence for a current-language rule. Canonicalization and composition
decisions require a version 6 golden fixture in the same change; focused Go
tests cover diagnostics, internal invariants, and cases that do not define
emitted bytes.

| Normative area | Current v6 golden evidence | Focused implementation evidence |
| --- | --- | --- |
| Manifest, closed fields, retired sources | 059, 091, 097 | `internal/resource`: `TestManifestIsClosedAndV6Only`, `TestScalarsAreTypedByTheirField`; `internal/compile`: `TestLegacySourcesRequireTheArchivalLanguage`, `TestProjectManifestRejectsUnknownFields` |
| Kind and identity from paths; reference kinds | 059, 093, 095 | `internal/resource`: `TestKindAndIdentityComeFromThePath`, `TestReferencesAreKindCheckedPaths` |
| Source membership as the manifest closure | 068, 071 | `internal/resource`: `TestLoadIsTheManifestClosure`, `TestMissingReferencedDocumentFails`; `internal/packages`: `TestSourceMembershipIsTheManifestClosure`; `internal/compile`: `TestSourceDigestIgnoresGeneratedAndUnreferencedFiles` |
| Frontmatter grammar | 069, 090 | `internal/tferlex`: all tests; `internal/lsp`: `TestServerDiagnostics` (line-accurate diagnostics) |
| Schema-directed scalars and numeric tokens | 066, 089, 094, 098 | `internal/tferlex`: `TestPlainScalarsAreUntypedText`, `TestQuotedIsAlwaysAString`; `internal/resource`: `TestScalarsAreTypedByTheirField`; `internal/resolve`: `TestNamedContextValuesMaterializeDefaultsAndPreserveScalarTypes` |
| BOM, CRLF, and Unicode | 069, 070 | `internal/jsonx`: escape, UTF-8, and surrogate tests |
| Plugins, artifacts, and the marketplace index | 059, 060, 063, 092, 097 | `internal/compile`: `TestPluginArtifactsRenderOneModeEach`, `TestMarketplaceIndexListsEveryArtifact`, `TestPluginValidatesOnlyItsModesContext` |
| Emitted names: grammars and build-wide uniqueness | 086, 087 | `internal/compile`: `TestSkillNameCollisionsFailClosed`, `TestInvalidHostNamesAreNeverRewritten`, `TestTargetNativeNameCollisionsFailClosed` |
| Routing metadata never enters instruction bodies | 059, 065, 088, 097 | `internal/compile`: `TestDescriptionNeverEntersInstructionBodies` |
| Implied capabilities and skill bindings | 059, 061 | `internal/resource`: `TestNormalizationDerivesWhatAuthorsDoNotWrite` |
| Additive skill extension | 061, 062, 080, 081, 082, 083, 096 | `internal/resource`: `TestExtensionRulesFailClosed`; `internal/compile`: `TestExtensionFlattensPerModeAndOverridesTheBase`, `TestSealedSkillCannotBeExtended` |
| Skill-held context and independence | 063, 064, 084, 085 | `internal/compile`: `TestSkillOwnedContextTravelsWithTheSkill`, `TestAgentDependentSkillShipsOnlyThroughAnAgent` |
| Objectives and built-in text | 065, 099 | `internal/resource`: `TestLoadIsTheManifestClosure`, `TestNormalizationDerivesWhatAuthorsDoNotWrite` |
| Pack compatibility report | 067 | `internal/compile`: `TestCompatibilityReportNamesCompetingPlugins` |
| Embedding, promotion, local resolution, and equal-path diamonds | 061, 096 | `internal/resolve`: promotion, ambiguity, duplicate-embed, sealed-diamond, and modifier-ambiguity tests |
| Required presence and sealed mutability | 096 | `internal/resolve/presence_test.go`, `internal/resolve/sealing_test.go`; `internal/resource`: abstract-binding tests |
| Structural interfaces and internal visibility | 071 | `internal/resolve`: `TestStructuralInterfaceSatisfaction`; `exposure_test.go` |
| Nominal context refinement and structural member validation | 064, 066, 074 | `internal/resolve/typed_context_test.go`, `schema_fields_test.go` |
| Allow-list intersection and fail-closed empty intersections | 074, 099 | `internal/resolve/gating_test.go` |
| Invocation modes and mode-scoped requirements | 062, 074, 092 | `internal/resolve/variants_test.go`, `gating_test.go`; `internal/compile`: `TestNeutralVariantFanout`, `TestPluginArtifactsRenderOneModeEach` |
| Tools as declared extern imports; plugin `mcp.json` at link | 074 | `internal/resolve`: tool declaration and schema tests; `internal/deploy`: `TestPluginMCPConfigurationIsMaterializedOnlyAtLink`, `TestPluginLinkRequiresEveryBuiltMode`, `TestPluginLinkRefusesCredentialForwarding` |
| Trust metadata, external signatures, and required-signature failure | 072, 073 | trust publication exercised through conformance; signature maps remain outside source roots |
| Offline locked packages, exports, and deterministic packing | — | `internal/packages`: `TestRestoreMaterializesTransitiveGraphForOfflineBuild`, `TestReferenceToUnexportedDependencyResourceFails`, `TestReferenceToUndeclaredPackageFails`, `TestPackIsByteDeterministic` |
| Unlinked build identity and deployment-only link changes | 059 | `internal/compile`: `TestBuildIsUnlinked`, `TestSourceDigestUnaffectedByResolverNormalization`; `internal/deploy`: linked-output ownership and tamper tests |
| Import | — | `internal/importer`: all tests |
| Setup wizard output | 058 | `internal/scaffold`: `TestGeneratedTreeCompilesWithOrdinaryCompiler`, `TestScaffoldProducesDeterministicTree` |
| Committed reference and self-host output | Helio `dist/`; maintainer `dist-maintainer/` | `internal/compile`: `TestHelioParityWithCommittedOutput`, `TestDeterministicRebuild`; `make selfhost-check` |

Rows without a golden fixture define no emitted bytes of their own or depend on
state the fixture runner does not model (restored package trees, deployment
files); their focused tests are the evidence.

## Maintenance rule

When a normative change lands:

1. update the specification and ADR first;
2. add or update the matrix row;
3. add a version 6 fixture for canonicalization or composition changes;
4. add focused tests for diagnostics and internal invariants; and
5. regenerate digests only through `go test ./conformance -update`.
