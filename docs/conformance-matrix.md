# Specification evidence matrix

This matrix connects the normative version 4 specification to executable
evidence. It is a review aid, not a second specification: when text and tests
disagree, `docs/specification.md` wins and both the implementation and evidence
must be corrected.

Current conformance fixtures omit the manifest `language` field. Fixtures marked
`legacy-v3` are archival byte regressions and never count as the only evidence for
a current-language rule. Canonicalization and composition decisions require a v4
golden fixture in the same change; focused Go tests cover diagnostics, internal
invariants, and cases that do not define emitted bytes.

| Normative area | Current v4 golden evidence | Focused implementation evidence |
| --- | --- | --- |
| Source versions, closed fields, duplicate keys | 034, 037 | `internal/resource`: `TestUnknownFieldRejected`, `TestDuplicatePropertiesAndNestedContextKeysRejected`, `TestDefaultLoaderRejectsLegacyV3` |
| `.tfer` fences and bodied kinds | 039, 043, 044 | `internal/resource`: `TestTferMissingOpeningFenceRejected`, `TestTferMissingClosingFenceRejected`, `TestMultimodalTferSkillRejectsBody` |
| `.tfer` BOM, CRLF, trailing-newline, and Unicode semantics | 045, 046, 047, 048 | `internal/resource` loader tests; `internal/jsonx`: escape, UTF-8, and surrogate tests |
| Schema-directed scalar parsing and canonical JSON number tokens | 027, 036, 049 | `internal/resolve`: `TestNamedContextValuesMaterializeDefaultsAndPreserveScalarTypes`; `internal/jsonx`: `TestNumberTokensPreserved`, `TestCanonicalCompact` |
| Embedding, promotion, local resolution, and equal-path diamonds | 038, 042 | `internal/resolve`: promotion, ambiguity, duplicate-embed, sealed-diamond, and modifier-ambiguity tests |
| Required presence and sealed mutability | 029, 038 | `internal/resolve/presence_test.go`, `internal/resolve/sealing_test.go`; `internal/resource`: abstract-binding tests |
| Structural interfaces and internal visibility | 040 | `internal/resolve`: `TestStructuralInterfaceSatisfaction`; `exposure_test.go` |
| Nominal context refinement and structural member validation | 027, 028, 032, 033, 036, 041, 042, 044 | `internal/resolve/typed_context_test.go`, `schema_fields_test.go` |
| Allow-list intersection and fail-closed empty intersections | 030 | `internal/resolve/gating_test.go` |
| Invocation modes and mode-scoped requirements | 031 | `internal/resolve/variants_test.go`, `gating_test.go`; `internal/compile/target_variant_test.go` |
| Tools as declared extern imports | 031 | `internal/resolve`: tool declaration/schema tests; `internal/deploy/deploy_test.go` |
| Target and dispatch-name uniqueness | 035 | `internal/compile/target_context_test.go` |
| Cross-platform digest path ordering | 050 | `internal/compile`: deterministic rebuild and parity tests |
| Trust metadata, external signatures, and required-signature failure | 051, 052, 053 | trust publication exercised through conformance; signature maps remain outside source roots |
| Offline locked packages and deterministic packing | — | `internal/packages/packages_test.go` |
| Unlinked build identity and deployment-only link changes | 031, 040 | `internal/compile/deployment_test.go`, `internal/deploy/deploy_test.go` |
| Committed reference and self-host output | Helio `dist/`; maintainer `dist-maintainer/` | `internal/compile`: `TestHelioParityWithCommittedOutput`, `TestDeterministicRebuild`; `make selfhost-check` |

## Maintenance rule

When a normative change lands:

1. update the specification and ADR first;
2. add or update the matrix row;
3. add a v4 fixture for canonicalization or composition changes;
4. add focused tests for diagnostics and internal invariants; and
5. regenerate digests only through `go test ./conformance -update`.
