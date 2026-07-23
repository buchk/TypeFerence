# 0017 — Tools as extern declarations

**Status:** Accepted (2026-07-23)

## Context

Skills reference runtime tools — a vault reader, an API client — and today those
tools "float": a skill hopes the tool is available at runtime, with no typed
relationship. The root cause is that a tool has only a runtime **body** (the code
that runs, off in deployment) and no compile-time **declaration**. It is the same
untyped-reference problem ADR-0013 fixed for context, one layer over.

There is also a vocabulary question. Checked against the MCP tools specification:
MCP uses **"capabilities"** for feature negotiation (a handshake) and **"tool"** for
an executable callable, and it *fuses* contract and implementation into that one
word. TypeFerence deliberately keeps them separate — which resolves "are we blending
skills and tools?": no, they are siblings.

## Decision

1. **`tool` is a first-class kind: an extern / header.** You author the tool's
   *signature* — its `id`, interface (`inputSchema`/`outputSchema`), and the
   auth/scope *shape* it demands — in the source. It carries **no context**. The
   *body* (the code that actually reads the vault or calls the API) is implemented
   outside the compilation unit, at runtime, by whoever deploys — exactly like a C
   header, a `.d.ts`, or a Go interface for an external service. This reconciles
   "the user writes the tool" (the body) with "tools are authored natively" (the
   declaration): different halves.

2. **The trio, un-fused (the object model's method layer):**
   - **capability** — the contract ("what"); the *signature* half of an MCP tool.
   - **skill** — **model**-fulfillment of a capability (the LLM does it via
     instructions/context); *no MCP equivalent*.
   - **tool** — **code**-fulfillment of a capability (an extern; the *implementation*
     half of an MCP tool).

   Skill and tool are **siblings** — two fulfillment modalities of one capability,
   not a blend. Previously only the model sibling existed; `tool` completes the pair.

3. **Skills declare typed tool dependencies (`requiresTools`), compile-checked.** A
   skill (or variant) declaring `requiresTools: [acme/tools/vault-reader@1.0.0]` is
   verified at build: the tool is declared and its interface shape-matches. Runtime
   hope becomes a build error. TypeFerence still never runs the tool.

4. **One tool, auth/scope varies per scoped skill extension.** A tool is declared
   once; each extension (an ADR-0012 variant) binds it with a different **scope and
   credential *reference***, never the secret. The `manual`/personal extension binds
   personal scope; the `a2a`/governed extension binds governed scope — and a governed
   extension can be *required* to bind a governed-scope tool, enforced structurally
   (the same "trust boundary is a type" move as ADR-0013 refinement). This matches
   the MCP model, which says the tool set "MAY vary by the authorization presented on
   the request — credentials are per-request input, not connection state."

5. **Boundary rule:** TypeFerence types the **extern** (declaration, dependency,
   scope-binding); it does not own the **body** (implementation, secrets, execution).
   Same line as "TypeFerence embeds no LLM provider."

6. **MCP naming crosswalk (also referenced by ADR-0018):** our **capability →** an
   MCP **tool** (signature); MCP **"capabilities"** is a transport-layer negotiation
   handshake *below* the TypeFerence layer and is **not** our `capability`. We keep
   `capability` internally (it does coherent Go-interface-method work, and `interface`
   is already a separate kind) and publish this crosswalk rather than renaming.

## Consequences

- Tools stop floating: every skill→tool dependency is declared and shape-checked at
  build; no runtime "hope it's there."
- The method layer of the object model is complete and symmetric: one contract
  (capability), two implementations (skill = model, tool = code).
- Auth is expressed as typed scope/credential references per extension; secrets never
  enter the definition, consistent with the no-credentials boundary.
- Runtime tools (Door B, ADR-0019) are typed here but implemented by the deployer.

## Alternatives considered

- **Keep tools implicit/hoped-for at runtime.** The floating status quo; no
  compile-time enforcement. Rejected.
- **Model a tool as just a capability with an "external" flag.** Understates it: a
  tool needs an auth/scope shape and is the *code-fulfillment* sibling of a skill, not
  merely a marked capability. Rejected in favor of a first-class extern kind.
- **Rename `capability` to avoid the MCP collision.** Churns a core, internally
  coherent word (and collides with the existing `interface` kind). Rejected in favor
  of a documented crosswalk.
