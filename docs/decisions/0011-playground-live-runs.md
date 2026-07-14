# 0011 — Playground equivalence console: no secrets in the browser

## Status

Accepted. Extends ADR-0010; implementation in progress on
`feature/playground-run` (an interim bring-your-own-key Run tab exists on
that branch as scaffolding and will be replaced before merge).

## Context

The playground (ADR-0010) shows what TypeFerence compiles but not what the
compiled definition *does*. The natural extension — fire the compiled agent
at real models and compare — is also the project's central question: BETH
(ADR-0009) exists because the same declared intent, run on different hosts,
may or may not behave equivalently.

A first implementation did this with bring-your-own-key adapters: paste an
Anthropic/OpenAI/Gemini/GitHub Models key into the page, requests go directly
from the browser to the provider, keys never persist unless opted in, and a
CSP pins `connect-src` to the four provider hosts. It worked, and it was
still wrong. Asking visitors to paste provider credentials into a hosted page
is exactly the anti-pattern this project's trust posture argues against: the
"keys never leave your browser" claim is unverifiable without auditing the
served JavaScript on every load, and one compromised deploy would turn the
page into a key harvester. A project whose pitch is provenance and
fail-closed trust should not train users to paste secrets into web pages.

There is also no real OAuth escape hatch: no target provider offers
third-party OAuth for API access, GitHub Copilot has no public API (and the
token exchange unofficial clients use violates GitHub's terms), and a backend
proxy would route other people's keys and prompts through project
infrastructure — strictly worse.

## Decision

The playground never asks for a secret. The Run surface becomes a **BETH
operator console** that keeps every credential in the user's own terminal:

1. **Pack in the browser.** The wasm bridge grows a `pack` entry point over
   the existing `eval.Pack`, laying out scenario × surface cells for the
   compiled definition (scenario files ship with the examples).
2. **Collect by copy and paste.** Each cell renders as a card: copy the
   prompt, run it in a real host (Claude Code, Copilot, Cursor, a raw API
   call — the operator's choice, with the operator's own keys), paste the
   response back. This is not a workaround; a human operator collecting one
   attested response per cell is BETH's defined collection model (ADR-0009).
3. **Export the run.** The console assembles the pasted responses into the
   run-directory layout and downloads it as a `.tar.gz` (dependency-free tar
   writer plus the browser's native gzip `CompressionStream`).
4. **Score locally.** The console prints the exact
   `typeference equivalence score <run> --live` command; the judge key lives
   in the local environment, where keys belong.
5. **Visualize the scorecard.** Dropping the resulting `scorecard.json` back
   onto the page renders adherence per surface, cross-surface agreement, and
   every divergence. Reading a local file requires no trust.

## Consequences

- The full BETH loop — author, compile, pack, collect, score, inspect —
  becomes walkable from a browser tab, and the only step that touches a
  credential happens in the user's terminal against the provider's own CLI
  or API.
- The interim BYOK adapters (`providers.js`) and key UI are deleted before
  merge; the verified per-provider transport knowledge they encode (CORS
  behavior, SSE wire formats) is preserved in this ADR's history should a
  legitimate need return.
- The console must not blur illustration into measurement: it emits and
  consumes BETH's own run-directory and scorecard shapes rather than
  inventing a parallel result format.
- Manual paste-back limits collection volume. That is BETH's existing,
  documented trade-off (one observation per surface, operator-attested), not
  a new one.

## Alternatives considered

- **Bring-your-own-key direct calls** (built, then rejected): unverifiable
  trust ask; normalizes pasting API keys into web pages; one bad deploy from
  being harmful.
- **OAuth/SSO to providers**: not offered for third-party API access by any
  target provider.
- **A backend proxy**: centralizes other people's keys and prompts; worse on
  every axis this project cares about.
- **Keeping BYOK behind an "advanced" toggle**: still trains the behavior;
  the honest console makes the same demo possible without it.
