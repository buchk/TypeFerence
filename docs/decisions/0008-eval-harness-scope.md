# 0008 — Behavioral eval harness: scope and design

**Status:** Accepted (2026-07-08)

## Context

The project's stated long-term objective is behavioral equivalence across hosts,
measurable and governable — and nothing in the repository measured behavior at all.
This ADR records the scope decisions behind the first honest cut (`typeference
eval`, `go/internal/eval`, `evals/`).

## Decisions

1. **Go implementation only, for now.** The eval harness is tooling, not spec
   semantics: it changes no compiled bytes and needs no spec text beyond honest
   framing. It lives in the Go implementation because that is the distributed
   static binary; the C# reference implementation stays focused on core semantics.
   Mirroring it in C# is possible later if evals become part of the conformance
   story.
2. **Dry-run is the default and the CI mode.** Without `--live`, the command
   validates scenarios against the resolved source and emits the exact Messages
   API request payloads without any network call. This keeps CI hermetic and makes
   the harness's behavior inspectable byte-for-byte before anyone spends tokens.
   `--live` requires `ANTHROPIC_API_KEY` from the environment and fails with a
   clear diagnostic otherwise.
3. **Raw HTTPS via the standard library, not the Anthropic Go SDK.** The official
   SDK is normally the right choice, and this deviation is deliberate: (a) the
   repository's dependency policy (ADR-0003) is stdlib-first, and the harness
   needs exactly one endpoint (`POST /v1/messages`); (b) the harness's defining
   feature is emitting the *exact request bytes* in dry-run mode, which requires
   owning serialization — `internal/jsonx` already produces deterministic JSON;
   (c) CI never performs network calls, so most SDK value (retries, streaming
   helpers, typed errors) is unexercised. The payloads follow the documented wire
   format: `anthropic-version: 2023-06-01`, default model `claude-opus-4-8`,
   adaptive thinking, and structured outputs (`output_config.format`) for the
   judge. Revisit if the harness grows streaming, batching, or retry needs — at
   that point the SDK earns its place and gets its own ADR.
4. **Judge design.** Grading uses a second model call constrained by a JSON schema
   (one verdict per rubric item, `passed` plus `reasoning`). The judge is
   instructed to grade literally and independently per item. The report is a
   deterministic-shaped JSON document; live mode exits 1 when any item fails so
   pipelines can gate on it.
5. **The harness measures the definition, not a deployment.** The executor's
   system prompt is the neutral instruction surface (rendered agent instructions,
   focused skill instructions, context file content from the source root). It does
   not emulate any host's prompt assembly or tool loop. Documentation states
   plainly that a pass is an adherence signal, not behavioral equivalence
   (see `evals/README.md`).

## Alternatives considered

- **Anthropic Go SDK** — rejected for now; see decision 3.
- **Evaluating compiled bundles instead of source trees** — rejected: the neutral
  target does not embed context-file content, so a compiled directory alone cannot
  reproduce the instruction surface. Resolving from source is deterministic and is
  the same definition. (Noted as design feedback alongside ADR-0006's context-drift
  observation.)
- **Deterministic string-match assertions instead of an LLM judge** — rejected as
  the primary mechanism: rubric items are behavioral ("does not fabricate a
  decision"), not lexical. The judge's noise is acknowledged in the docs rather
  than hidden.
