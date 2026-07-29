# repo-agent example

A small corpus that exercises the object-model features from ADRs 0012–0019 in
one place. Build it with the Go implementation:

```sh
typeference validate examples/repo-agent
typeference build examples/repo-agent --target neutral --out out
typeference inspect acme/repo-agent@1.0.0 --source examples/repo-agent
```

What each piece demonstrates:

- **Exposure / visibility (ADR-0015).** `capabilities/repository-status` is
  `visibility: exposed`, so the resolved skill is part of the agent's public
  callable surface. A native callable card is materialized only after a
  deployment endpoint/provider is linked.
- **Mode variants (ADR-0012).** `skills/repository-status` declares `variants`
  for `pipeline`, `manual`, and `a2a` instead of a single `instructions`. The
  neutral bundle emits a `variants` member with each mode's additive
  requirements; build does not flatten them.
- **Tools as extern (ADR-0017).** `tools/vault-reader` is a `tool` declaration
  (an extern header). The skill's `requiresTools` names it, checked at build and
  required to have a provider binding at link. The tool's *body* is implemented
  by the deployer, not TypeFerence.
- **Typed context + refinement (ADR-0013).** `context-types/governed-cast`
  embeds `cast-of-characters` (refinement). The skill `requiresContextTypes:
  [cast-of-characters]`; the agent holds `notes/team` — a `governed-cast` — and
  a governed cast *is a* cast, so the requirement is satisfied structurally.
- **Schema-directed fields (ADR-0020).** `governed-cast` declares a native
  `fields` mapping requiring `owner`; `notes/team` carries it under `values`.
  Remove it and the build fails with a missing-required-field
  error — required fields accumulate across the refinement closure.
- **Sealing (ADR-0016).** `profiles/repo-defaults` binds the status skill
  `sealed: true`. An agent embedding the profile may extend around it but cannot
  override or rebind the sealed capability.
- **`.tfer` format (ADR-0013).** `notes/team.context.tfer` is authored as
  frontmatter (typed fields) plus a markdown body (the roster content).

Change the corpus to see the type system push back: drop `context:` from the
agent and the `requiresContextTypes` check fails; add a second binding of
`repository-status` on the agent and sealing rejects the override; delete the
`vault-reader` tool and `requiresTools` fails.
