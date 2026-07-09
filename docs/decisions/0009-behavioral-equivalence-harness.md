# 0009 — BETH: the behavioral equivalence test harness

**Status:** Accepted (2026-07-09)

## Context

ADR-0008's eval harness measures the **definition**: it renders the neutral
instruction surface itself and calls a model API directly. It deliberately does
not emulate any host's prompt assembly, skill loading, or tool loop — so it
cannot say anything about what the compiled artifacts do inside Codex, Copilot,
Cursor, or another AGENTS.md-reading host. The project's stated long-term
objective — behavioral equivalence across hosts, measurable and governable —
still had no measurement instrument on the deployment side.

BETH (Behavioral Equivalence Test Harness, CLI verb `typeference equivalence`)
is that instrument: it runs the same scenario corpus against the **compiled
target bundles** as real hosts consume them, and scores cross-surface agreement.

## Decisions

1. **Two subcommands, manual-first.** `equivalence pack` compiles each
   scenario's agent into every requested target bundle and lays out one *cell*
   per (scenario × surface): a self-contained workspace directory plus
   `PROMPT.txt` and frozen scenario metadata. A human (or any automation the
   operator owns) opens a host with the cell workspace as its working
   directory, submits the prompt, and saves the final response as
   `response.md` (optionally recording the host and model in `runtime.json`).
   `equivalence score` judges collected responses and emits a scorecard.
   The harness ships **no host-execution runner**: vendor CLI flags churn
   quickly, neither CLI was invocable on the development machine, and shipping
   an untested runner would violate the repository's no-fabricated-claims
   rule. Example one-liners for `claude -p` and `codex exec` are documented as
   suggestions, marked unverified.
2. **Cells embed their scenario.** `cell.json` freezes the task and rubric at
   pack time, so a run directory is self-contained and immune to later edits
   of the scenario corpus. Scoring reads only the run directory.
3. **Context files are materialized into each workspace, and that is a
   finding.** Compiled bundles reference `context/*.md` paths they do not
   contain (see ADR-0006's context-drift observation and ADR-0008's
   bundle-evaluation rejection). A bundle alone is therefore not a deployable
   instruction surface; `pack` copies the referenced context files from the
   source root into the workspace and records the list in `cell.json`. This is
   recorded design feedback: target adapters may need a context-emission mode.
4. **The judge is reused, and judge provenance is recorded.** Scoring reuses
   the ADR-0008 judge (same payload builder, same structured-output schema,
   same literal-grading instructions). Three judge paths exist, in precedence
   order per cell: a pre-existing `judge-response.json` (any operator or model
   may act as judge; the scorecard records `judge: file`); `--live` API
   grading (records `judge: anthropic:<model>`); otherwise the exact judge
   request payload is emitted (`judge-request.json`) and the cell is reported
   unjudged. Dry-run-by-default is preserved from ADR-0008.
5. **Equivalence and adherence are reported separately.** Two surfaces that
   both fail a rubric item are *equivalent* (they preserved — or lost — intent
   identically) even though neither *adheres*. The scorecard reports, per
   rubric item, the per-surface verdicts, an agreement flag across judged
   surfaces, and separate totals: adherence rate per surface and an agreement
   rate across surfaces. Divergences are listed explicitly with judge
   reasoning. `score` exits 1 when any judged rubric item diverges across
   surfaces or fails anywhere, so pipelines can gate on either signal.
6. **Neutral is a control surface.** The neutral bundle's `AGENTS.md` is
   rendered by the same function as the codex bundle's, so a neutral cell run
   in the same host as a codex cell should agree with it almost by
   construction. Including it (default `--target all`) gives the scorecard a
   built-in control: neutral/codex divergence signals judge or sampling noise
   rather than adapter drift.
7. **One response per cell.** A cell holds one observation. Statistical
   treatment (repeated runs, inter-judge agreement) belongs in separate run
   directories aggregated by the operator; the scorecard states plainly that a
   single run is an observation, not a proof.
8. **Go implementation only**, inside `go/internal/eval` (shared scenario
   loader, judge builder, verdict parser, and backend), for the ADR-0008
   reasons: tooling, not spec semantics; no compiled bytes change.
9. **Determinism.** `pack` output is byte-deterministic for identical source
   and scenarios (no timestamps; workspace digests via the existing directory
   hasher; `internal/jsonx` for all JSON). `score` without `--live` is a pure
   function of the run directory. Both properties are tested.

## What a scorecard establishes — and what it does not

A green scorecard means: on these scenarios, one judged response per surface,
collected on whatever hosts the operator used, was graded as making the same
pass/fail decisions. It does not establish equivalence on other tasks,
phrasings, models, or dates, and LLM judging is itself noisy. The instrument's
value is longitudinal: the same corpus re-packed and re-scored across adapter
changes, host versions, and models turns "behavioral equivalence" from an
aspiration into a tracked number.

## Alternatives considered

- **Shipping `claude`/`codex` exec runners** — rejected for v1; see decision 1.
  Revisit when a runner can be exercised in CI or on the development machine;
  the cell layout is already runner-friendly (workspace + PROMPT.txt in,
  response.md + runtime.json out).
- **A new top-level scenario schema for equivalence runs** — rejected: the
  ADR-0008 scenario format (task + rubric) is exactly the needed shape, and one
  corpus serving both harnesses is the point — the definition-side and
  deployment-side measurements stay comparable.
- **Scoring by diffing response texts across surfaces** — rejected: surface
  texts legitimately differ; the objective is preserved *intent*, which is what
  the rubric encodes. Textual similarity would reward parroting and punish
  benign variation.
- **A separate `beth` CLI verb** — rejected: cute, but the command surface
  stays full-word and self-describing (ADR-0002's spirit). BETH remains the
  informal name in prose.
