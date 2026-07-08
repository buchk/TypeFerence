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
