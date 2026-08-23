# v5 Closure + Deterministic Setup Wizard — Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.
> Spec changes follow the `typeference-spec-changes` skill process (non-negotiable order).

**Goal:** Land one bundled breaking closure — single `.tfer` source format + "no untyped behavioral prose" rule (v4→v5) — then build the deterministic setup wizard (`typeference init` + browser wizard) on top of it, with helio as the playground's default-on-load example. The wizard doubles as the acceptance corpus proving the v5 closure is livable.

**Architecture:** Two sequential phases in separate branches off `main`. Phase 1 is a specification-first language change (spec → ADRs → fixtures → implementation → determinism). Phase 2 adds one pure generator (`AnswerSet → Scaffold() → SourceTree`) consumed by two front doors (CLI `init`, wasm `scaffold`), pinned by versioned answer sets and golden fixtures reusing the existing directory-digest canonicalization.

**Tech Stack:** Go 1.x (module `github.com/buchk/TypeFerence/go`), Go `js/wasm` bridge, static JS playground (no framework, dependency-free per ADR-0010), Make targets (`make test`, `make conformance`, `make selfhost`, `make playground`).

---

## Current context / assumptions

- Repo root: `C:\Users\albuc\Documents\TypeFerence`. Verify with `git status` before starting; branch `main`, clean at session start.
- CLI commands today (`go/cmd/typeference/main.go:39-61`): `validate, build, pack, restore, update, link, inspect, diff, publish, eval, equivalence, version`. **No authoring/scaffold command exists.**
- Playground exists at `web/playground` (ADR-0010): unmodified compiler as wasm, `TypeFerence.compile` entry point, examples packed into `examples.json` by `go/cmd/playground-pack/main.go` via `make playground`.
- Equivalence console exists (ADR-0011): includes a dependency-free ustar+gzip tar writer in playground JS (reusable for tree download).
- Existing canonicalization: `typeference-directory-v1` digest over emitted target directories, recorded in `conformance/fixtures/`, regenerated only via `go test ./conformance -update`.
- Loader already rejects unknown fields (per-kind field map); bodies are legal only on skills and contexts. Remaining untyped doors: `workingNorms` (string list) and `description` on agents/profiles.
- Owner-settled positions (do NOT re-litigate): closed spec, no type inference ever, `.tfer` takes YAML indentation/comments/block-scalars + JSON scalar typing, no anchors/aliases/flow/multi-doc, bundle related breaking changes into one closure.
- Dead weight: stale C#/bin/obj under `src/`, `tests/`, `.vs/` — ignore them; Go is the sole implementation (ADR-0014).
- Every commit must pass `go test ./...` (from `go/`) AND `make conformance`.

---

## Phase 1 — v5 closure (branch: `feat/v5-closure`)

### Task 1.1: Amend the specification — `.tfer` format

**Objective:** Normatively define the single `.tfer` surface format in `docs/specification.md` before any code moves.

**Files:**
- Modify: `docs/specification.md` (surface-syntax section)

**Steps:**
1. Replace dual `.yaml`/`.tfer` acceptance with: `.tfer` is the sole source format. Cite YAML block-scalar chomping/indentation semantics verbatim (`|`, `|-`, `|2`) — not reinvented.
2. Specify scalar typing by syntax: quoted = string; digits = integer; true/false = bool; NO bare-word scalars; no implicit typing.
3. Forbid: anchors, aliases, tags, flow collections, multi-document streams. Each forbidden construct gets its own error case named in the spec.
4. Comments remain legal everywhere (they are inert metadata and never reach artifacts).
5. State the migration rule: existing `.yaml` sources convert mechanically; conversion is syntax-only, no semantic rewrite.

6. **Scalar type ruling (settled 2026-08-23):** `.tfer`'s scalar vocabulary is `string`, `integer`, `decimal`, `boolean` — deliberately boring, NOT Go's numeric shelf (`int` is implementation-sized; floats are a canonicalization swamp; Go is the reference realization, not the definition; no enums/sum types exist there anyway). Composites are equally standard: `list<T>`, `map<string,T>`, declared records, nominal `contextType`. Specifically:
   - `integer` is first-class and arbitrary-precision (canonical form = written digit string; digests trivially stable). Reference impl maps to `math/big.Int` at edges, bounded int64 internally where provably safe.
   - `decimal` is legal and canonicalized by preserving the written lexeme verbatim (like block scalars preserve prose). No arithmetic guarantees, no IEEE 754 semantics — `0.50` ≠ `0.5` because they are different canonical bytes. This dissolves the old YAML-boundary "integer vs number" question: v5 typing is syntactic, nothing guesses.
   - **No `float` type in v5.** Floating-point math is programming, not organizational-behavior declaration. Addable later as a new type name without breaking anything.
7. The scalar ruling is recorded in the format ADR alongside the rejected alternative "adopt Go's standard types" (rejected for portability/digest/spec-first reasons above).

**Verification:** Spec section reads standalone; every error case enumerated. No code changed yet.

---

### Task 1.2: Amend the specification — no untyped behavioral prose

**Objective:** Write the normative field-classification rule and kill `workingNorms`.

**Files:**
- Modify: `docs/specification.md` (resource model section)

**Steps:**
1. Add the normative sentence verbatim: **"a field is either typed context, a reference, or inert metadata; nothing else."**
2. Delete `workingNorms` from agents/profiles. Norms become held context: profiles declare `context: [<org>/context/<norms-id>@<version>]` against a declared `contextType` (e.g., a norm-collection type with `{statement: text}` members, richer shapes allowed).
3. Rule on `description`: legal ONLY if emission guarantees it lands solely in human-facing manifest/bundle metadata, never in instructions. If any emitter currently routes it model-facing, that path is removed in this closure (check emitters under `go/internal/`).
4. Agent-local context must also be declared/typed (owner decision) — confirm the spec already requires `contextType` on every context resource; close any gap.
5. Define migration: `workingNorms` entries become context resources of a norm contextType, referenced from the declaring profile.

**Verification:** Grep the spec for `workingNorms` — zero remaining occurrences outside the migration note.

---

### Task 1.3: Record ADRs

**Objective:** Capture decisions + rejected alternatives in the same change.

**Files:**
- Create: `docs/decisions/00XX-single-tfer-format.md` (next free number)
- Create: `docs/decisions/00XX-no-untyped-behavioral-prose.md`
- Modify: `docs/decisions/README.md` index

**Content requirements:** Context, Decision, Consequences, Alternatives considered. For the format ADR, rejected alternatives include keeping dual formats and reinventing block scalars. For the prose ADR: rejected alternatives include keeping `workingNorms` as deprecated-but-legal, allowing inferred types, and making `description` typed.

---

### Task 1.4: Convert the conformance corpus

**Objective:** All fixtures become valid v5 source; new fixtures cover the deleted doors.

**Files:**
- Modify: everything under `conformance/fixtures/` (mechanical `.yaml` → `.tfer` conversion)
- Create: fixture covering `workingNorms` → norm-context migration shape
- Create: fixture asserting rejection of each newly-forbidden construct (bare-word scalar, anchor, alias, flow collection, multi-doc, untyped behavioral string field)
- Create: fixture proving `description` stays inert (emitted to metadata only)

**Steps:**
1. Write the mechanical converter as a throwaway Go program or script; do not hand-edit dozens of files.
2. Regenerate digests ONLY with `go test ./conformance -update` — after the implementation compiles v5 (Task 1.6). Never hand-edit digests.

---

### Task 1.5: Update the implementation — parser/loader/emitters/LSP

**Objective:** Go implementation accepts exactly v5 and nothing else.

**Files (expected — locate precisely first with search_files):**
- Parser/loader under `go/internal/` (per-kind field maps: delete `workingNorms`, enforce field classification)
- Emitters (ensure `description` reaches metadata only)
- `go/cmd/typeference-lsp/` (diagnostics for new errors; `.yaml` reported as unsupported with pointer to migration)
- `go/cmd/playground-pack/` (example trees become `.tfer`)
- `examples/helio/` converted to `.tfer`; norms moved to held context

**Steps (TDD where sensible):**
1. Failing tests first for each newly-rejected construct.
2. Implement strict scalar typing + block scalars per spec citation.
3. Remove `workingNorms` handling; add norm-context composition path (reuses existing context machinery — no new semantics).
4. Update `examples/helio`; regenerate `dist/` via `typeference build` (never hand-edit).
5. Run `go test ./...` until green.

**Verification:** `cd go && go test ./...` passes; `make conformance` reproduces all regenerated digests; byte-comparison tests against `dist/` pass.

---

### Task 1.6: Selfhost + docs sweep

**Files:**
- `agents/maintainer/` resources if their definition uses `.yaml`/`workingNorms` (likely — maintainer AGENTS.md has working norms); regenerate via `make selfhost`
- `docs/whitepaper.md`, `README.md`, `web/playground/README.md`, quick start referencing helio
- `CHANGELOG.md`

**Verification:** CI-equivalent locally: `make test`, `make conformance`, `make selfhost` produces zero diff after commit.

**Commit cadence:** One commit per task, conventional messages, branch `feat/v5-closure`. PR when the whole phase is green.

---

## Phase 2 — Setup wizard + init (branch: `feat/setup-wizard`, rebased on Phase 1)

### Hard boundary (from review feedback — adopted wholesale)

```
AnswerSet ──► Scaffold() ──► SourceTree ──► { CLI fs writer | wasm bridge | tar writer | tests }
```

- **AnswerSet**: versioned input document `{ "schemaVersion": 1, "organization": {...}, "norms": [...], "team": {...}, "targets": [...] }`. Parsed with loader-grade strictness: declared schema, unknown fields are errors, no untyped channels (the wizard's own config is typed too).
- **Scaffold()**: pure function in `go/internal/scaffold/`. Knows NOTHING about browsers, disk, tar, stdout, wasm.
- **SourceTree**: in-memory ordered tree of `{path, bytes}`. Single consumer interface for every front door.
- **Scaffold digest** = the EXISTING `typeference-directory-v1` digest applied to the generated tree. No new canonicalization is invented.
- Reproducibility contract: **(generator version × AnswerSet schemaVersion) → bytes**, both stamped in an emitted scaffold manifest.

### Task 2.1: ADR — deterministic setup wizard

**Files:**
- Create: `docs/decisions/00XX-deterministic-setup-wizard.md` (extends ADR-0010; compatible since a wizard is authoring tooling, not runtime surface)

**Must contain:**
- One generator, two front doors; SourceTree boundary; digest reuse.
- Versioned answer schema; pinned-old-fixture reproducibility rule.
- Exit ramp: `typeference init --answers answers.json --verify <digest>`; mismatch prints diverging paths and exits nonzero; version mismatch prints generator vX.Y vs vX.Z with upgrade/download guidance (fail-closed but legible).
- **Explicit non-goals:** the wizard never owns TypeFerence semantics — no wizard-only constructs, no hidden metadata required to compile, nothing generated that a human couldn't hand-author.
- Rejected alternatives: JS-side scaffolding in the browser; flags-based answer passing (shell escaping undermines determinism); a parallel scaffold digest.

### Task 2.2: AnswerSet schema + parser

**Objective:** Strict, versioned input parsing.

**Files:**
- Create: `go/internal/scaffold/answerset.go`
- Test: `go/internal/scaffold/answerset_test.go`

**TDD steps:**
1. Failing test: valid minimal AnswerSet parses.
2. Failing test: unknown field → error naming the field (mirrors loader behavior).
3. Failing test: missing/wrong `schemaVersion` → explicit error.
4. Implement parser; reuse the loader's strict-field-map philosophy.
5. `go test ./internal/scaffold/ -v` green. Commit.

### Task 2.3: Scaffold() generator + golden fixtures

**Objective:** Answers → ordinary v5 source, deterministically.

**Files:**
- Create: `go/internal/scaffold/generate.go` (SourceTree type + pure generator)
- Test: `go/internal/scaffold/generate_test.go`
- Create: `conformance/fixtures/scaffold/<canonical-answer-set>/` — generated tree + expected directory digest (the **golden wizard fixture**)
- Create: `conformance/fixtures/scaffold/<pinned-v1-answer-set>.json` — kept forever to prove old answers still reproduce after future convention changes

**Fixture simultaneously proves (acceptance-corpus framing):**
✓ typed behavioral context · ✓ multilevel composition · ✓ profile embedding · ✓ slot resolution · ✓ target emission · ✓ compilation · ✓ deterministic source generation · ✓ deterministic artifacts · ✓ browser/CLI equivalence

**Non-goal enforcement (tested property, not promise):** compile the golden fixture through the ORDINARY compiler with zero wizard context (it must build), plus assert no reserved/wizard-only keys appear anywhere in the generated tree.

**Steps:**
1. Design the canonical answer set: org identity → 2–3 norm statements → enterprise→department→team→agent embedding chain → target list. Default flow teaches inheritance ONLY; the slot-collision lesson is opt-in (see Task 2.7) and excluded from the default fixture.
2. TDD the generator: failing digest test → implement → green.
3. Wire into the conformance runner so `make conformance` covers scaffold digests.
4. Generated `.tfer` carries explanatory comments (comments are inert; they survive).
5. Commit; `make conformance` green.

### Task 2.4: CLI front door — `typeference init`

**Files:**
- Modify: `go/cmd/typeference/main.go` (register subcommand)
- Create: `go/cmd/typeference/init.go` (or internal command package, matching existing layout)

**Behavior:**
- `typeference init --answers answers.json [--out DIR]` — writes SourceTree to disk.
- `--verify <digest>`: after writing, compute directory digest; on mismatch print which paths differ (path-set divergence vs content divergence distinguished) and exit nonzero. On version mismatch print both versions with guidance.
- Optional interactive prompt mode later; answers-file form is the committed contract.

**TDD steps:** failing end-to-end test (answers.json → tree → digest match → verify success; tampered byte → verify failure with correct message) → implement → green → commit.

### Task 2.5: WASM front door — `TypeFerence.scaffold`

**Files:**
- Modify: `go/cmd/typeference-wasm/main.go` (add `scaffold` entry point beside `compile`; same pattern, untouched internals)
- Modify: `Makefile` / `go/cmd/playground-pack/main.go` if example packing needs regeneration
- Verify: `make playground` builds; CI wasm gate stays green.

### Task 2.6: Browser wizard UI (presentation layer — OUTSIDE the determinism story)

**Files:**
- Modify: `web/playground/index.html`, `app.js`, `style.css`
- Reuse: existing tar writer from ADR-0011 work for tree download

**UX sequence (~4 steps, first-run teaches inheritance only):**
1. **Identity** — org name/id (ASCII/punycode rules surfaced inline).
2. **Norms** — 2–3 free-text statements. THE teaching moment: user writes plain prose; UI shows the named contextType it becomes ("This is behavioral prose, but it isn't untyped — engineeringNorm determines where it can be composed").
3. **Composition** — base→team→agent; live file tree highlights as the graph grows; click any file to see why it exists. Show "✓ Your agent inherited N resources."
4. **Targets** — choose hosts; compile live in-pane; show artifacts + digest side-by-side.

**Exit ramps:**
- Download generated tree (tar) AND `answers.json`.
- Print exact local reproduction command:
  `typeference init --answers answers.json --verify <digest>`
  Demo copy: *"Run this locally. TypeFerence refuses success unless your generated suite matches this browser session."*

**Opt-in second lesson (one click deeper, per feedback):**
> ✓ Your agent inherited 5 resources. Want to see what happens when teams disagree? [Show me]
→ deliberately introduces a slot collision and walks shallowest-wins/local disambiguation interactively.

**Presentation-layer discipline:** annotations/explanations derive from generator/compiler output wherever possible; never hand-key semantics to filenames in JS (that would be a second semantics layer). Helio loads by default on page open (ordering/packing choice in `playground-pack`/`app.js`); wizard reachable from the landing state.

**Compile-per-step latency:** measure wasm recompile time early (Task 2.5). If >~300ms feels sluggish, cache compiled module instance and recompile only dirty inputs — still real compiler runs, no fake validation.

### Task 2.7: Docs + marketing surfaces

**Files:**
- Modify: `README.md` quick start (wizard as entry point), `docs/whitepaper.md` if narrative references onboarding, `web/playground/README.md`, `CHANGELOG.md`
- Constraint: describe honestly as experimental reference implementation; no invented adoption/benchmarks/endorsements (AGENTS.md rule).

**Key sentence the whole feature optimizes around proving:**
> "We deleted all untyped behavioral fields — and onboarding got easier, not harder."

---

## Verification (whole plan)

Per-commit gates (both phases):
```bash
cd go && go test ./...
make conformance        # includes scaffold digests once Phase 2 lands
```
Phase 1 additionally: `make selfhost` zero-diff after regeneration; `dist/` byte-compare green.
Phase 2 additionally: `make playground` builds; manual walkthrough of the 4-step wizard in-browser producing a tree whose digest matches `typeference init --answers answers.json --verify` locally.

## Risks / tradeoffs / open questions

| Risk | Mitigation |
|---|---|
| Fixture churn twice if wizard constructs need spec tweaks | Wizard emits ONLY existing v5 constructs; if it can't, that's a spec bug to fix in Phase 1 terms |
| Cargo-cult scaffolds users don't understand | Annotated output + per-step live compile; collision lesson opt-in |
| Template matrix explosion | ~5 fixed questions, closed option catalog; free text only where a type absorbs it (norm bodies) |
| `--verify` fails for legit users on older CLIs | Explicit version negotiation in the error (generator vX vs website vY + upgrade path) |
| Playground scope creep past ADR-0010 ("no command surface") | Own ADR extends 0010; wizard is authoring tooling, not runtime |
| wasm recompile latency kills the live loop | Measure in Task 2.5; cache module, recompile dirty inputs only |
| `integer` vs `number` distinction cost at the YAML boundary | Open amendment candidate — decide during Phase 1 spec drafting, record in the format ADR |

## Sequencing summary

1. `feat/v5-closure`: spec (format + prose rule) → ADRs → fixtures → implementation → determinism green → PR.
2. `feat/setup-wizard`: ADR-0021-ish → AnswerSet → Scaffold() + golden fixture → `init` → wasm bridge → wizard UI (+ helio default-on-load) → docs → PR.
