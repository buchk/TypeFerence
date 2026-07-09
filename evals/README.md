# Behavioral evals

`typeference eval` is the first honest cut of measuring how a compiled agent
definition behaves: given a typed source tree and a scenario (a task prompt plus an
expected-behavior rubric), it runs the scenario against a model backend and has an
LLM judge grade each rubric item independently.

```
typeference eval examples/helio --scenarios evals/scenarios          # dry run
typeference eval examples/helio --scenarios evals/scenarios --live   # calls the API
```

- **Dry run is the default.** It validates every scenario file, resolves the agent
  and skill against the source, and emits the exact Messages API request payloads
  (executor and judge) without making any network call. CI runs dry-run only.
- **Live mode** (`--live`) reads `ANTHROPIC_API_KEY` from the environment, executes
  each scenario, judges it, prints a JSON report, and exits 1 if any rubric item
  fails. The default executor/judge model is `claude-opus-4-8` (`--model` overrides).
- `--out <dir>` writes payloads (and, live, responses and the report) to disk.

## Scenario format

```yaml
schemaVersion: 1
id: evals/payments-status-evidence
agent: helio/payments-repo-agent@1.0.0        # resource id in the source tree
skill: payments-repo-agent.repository-status  # optional dispatch name to focus on
task: |
  The user prompt sent to the agent.
rubric:
  - id: withholds-healthy-verdict
    requirement: What the response must actually do to pass this item.
```

Rubric items should be independently gradeable statements about the response text —
"does not declare the service healthy while a control signal is missing", not "is
good". The judge grades each item literally.

## What a passing eval establishes — and what it does not

Consistent with the design-boundaries section of the README and the specification's
own framing ("behavioral equivalence across hosts is a long-term conformance
objective, not a v1 compiler guarantee"):

- A pass means: **one model, on one day, judged this one response as adhering to
  this rubric.** It is a useful adherence signal, especially tracked over time and
  across definition changes.
- A pass does **not** establish behavioral equivalence across hosts, models, or
  targets. It does not establish that the agent will behave this way on other
  tasks, phrasings, or dates. LLM-judged grading is itself noisy; a single run is
  an observation, not a proof.
- The harness evaluates the **neutral instruction surface** (rendered instructions,
  skill instructions, context content) composed the same way for every backend. It
  does not reproduce any specific host's prompt assembly, tool loop, or truncation
  behavior — so it measures the definition, not a deployment.

The backend interface (`go/internal/eval/backend.go`) is deliberately small so
additional providers can be added; `anthropic` is the first implementation, and the
dry-run backend is what CI exercises.

## Behavioral equivalence (BETH)

`typeference eval` measures the definition; `typeference equivalence` (informally
BETH, the Behavioral Equivalence Test Harness — ADR-0009) measures the compiled
deployment surfaces. It runs the **same scenario corpus** against the actual target
bundles as hosts consume them, and scores whether the surfaces make the same
pass/fail decisions.

```
typeference equivalence pack examples/helio --scenarios evals/scenarios --out beth-run
# ... run each cell in a real host, save response.md ...
typeference equivalence score beth-run            # dry: emits judge payloads
typeference equivalence score beth-run --live     # judges via ANTHROPIC_API_KEY
```

`pack` lays out one **cell** per scenario × surface (default `--target all`:
neutral, codex, copilot, cursor):

```
beth-run/
  manifest.json                     # cells, workspace digests; no timestamps
  README.md                         # the collection protocol, restated
  cells/<scenario>/<surface>/
    cell.json                       # frozen task + rubric, agent, materialized context
    PROMPT.txt                      # the task, exactly as the user message
    workspace/                      # the compiled bundle + referenced context files
    response.md                     # you add: the host's final response
    runtime.json                    # you add (optional): {"host": "...", "model": "..."}
```

To collect a cell: open the host with `workspace/` as its working directory (Codex
and Claude Code read `AGENTS.md`; Copilot reads `.github/copilot-instructions.md`),
submit `PROMPT.txt` verbatim as the first message, and save the final response text
as `response.md` in the cell directory. Anything that automates this loop is fine —
suggested (unverified here) one-liners: `claude -p "$(cat PROMPT.txt)"` or
`codex exec "$(cat PROMPT.txt)"` from inside the workspace.

`score` reuses the eval judge per cell: a pre-seeded `judge-response.json` wins
(any operator or model may act as judge; recorded as `judge: file`), `--live` calls
the API, and otherwise the exact judge payload is written to `judge-request.json`
and the cell is reported unjudged. The scorecard (`scorecard.json` / `scorecard.md`)
reports **adherence** (per surface) and **agreement** (across surfaces) separately —
two surfaces failing an item identically are equivalent but non-adherent — and lists
every divergence with the judge's reasoning. Exit 0 only for a green scorecard —
one judged response per surface, all agreeing, none failing (ADR-0009); exit 1 on
any divergence, failure, or incomplete coverage (so a dry or partly-collected run,
which observes nothing conclusive, is not mistaken for a pass).

A green scorecard is one observation per surface on one day — an instrument reading,
not a proof. Its value is longitudinal: re-pack and re-score the same corpus across
adapter changes, host versions, and models, and the README's "behavioral
equivalence" objective becomes a tracked number.
