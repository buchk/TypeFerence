# Design note: version 7 keep / change / add / cut inventory

Status: inventory drafted 2026-10-06 against commit `0391377`. Input to a
framing ADR and a version 7 specification; it decides nothing by itself.

## The frame

TypeFerence becomes an **authoring and reuse layer for GitHub Copilot
plugins**. It compiles an organization's skills, agents, and MCP servers into
ordinary Agent Plugins 1.0 directories plus a Copilot marketplace. Agent
Plugins 1.0 is the default output; Copilot-only features are explicit opt-ins
emitted into `com.github.copilot/`. Copilot and GitHub install, host, enable,
authenticate, and run everything. TypeFerence does not.

The motivating deployment is an enterprise AI suite with two paths into the
same packaged expertise:

- **Interactive:** people install plugins from one internal marketplace.
- **Automated:** pipelines run Copilot CLI with a skill against trusted
  evidence. A runner validates the skill's result against a versioned schema
  before it is stored or acted on.

TypeFerence owns only the packaged-expertise part of that suite. Policy, usage
reporting, pipeline templates, runners, and integrations are runtime systems
and stay outside it.

The test for every item below is whether it serves one of these four:

1. reuse that Markdown cannot express (extension, templates, shared context);
2. checks the plugin format cannot express (names, servers shipped with
   skills, independence, output contracts);
3. one marketplace build with one version of each package;
4. a reviewable diff of every emitted byte.

## Revisions to earlier conclusions

- **Keep multi-plugin packages and plugin documents.** An earlier idea was one
  plugin per root. Real marketplace repositories hold several plugins plus a
  catalog in one repository, and v6's model (a package lists zero or more
  plugins; with none it is a library; a marketplace package lists its
  dependencies' plugins) already matches that. The three root kinds already
  exist as library, plugin package, and marketplace package. The idea of
  mirroring the output layout in the source is dropped too, because shared
  skills across plugins don't mirror cleanly.
- **Keep pipeline mode and output schemas.** These were cut candidates. The
  automated path depends on both: a skill must render strictly for a runner
  and return a result in a known shape.

## Keep

| Area | Why it stays | Notes |
| --- | --- | --- |
| Closed `.tfer` grammar, schema-directed scalars (`tferlex`) | Determinism and clear errors | Unchanged |
| Path-derived identity and references | No ceremony; collisions are build errors | Unchanged |
| Profiles and agent embedding | Enterprise rules, then team profile, then agent | Promotion, shallowest-wins and ambiguity rules unchanged |
| Skill extension (`extends`) | Team specialization of shared skills | Additive, because that's the simplest useful rule, not as protection; base for template instances. `sealed` is cut. |
| Context, context types, held context | Enterprise rules and team facts written once | Simplified (see Change); becomes model-visible through field references (Add 1) and can render as a file (Add 5) |
| Skill independence (derived) | Slash-command skills must be complete | Gains the explicit agent-only category (Add 4) |
| Invocation modes `manual` and `pipeline` | One skill, two renderings: people and runner | `a2a` is removed |
| Capabilities, bindings, `required` | Output contracts, rebinding, family report | `visibility` is removed |
| `inputSchema` and `outputSchema` | Runner result contracts | Must reach output (Add 6) |
| `agent-plugin` target and `marketplace.json` | The product | Already emits Copilot's marketplace format |
| Organization marketplace (ADR-0034), `validate --candidate` | Cross-team name and version guarantees | Unchanged |
| Packages: `pack`, `restore`, `update`, lockfile | Extension needs base-skill source across repositories | Azure Artifacts is the primary feed, with JFrog Artifactory next; filesystem, HTTP, JFrog and Azure Artifacts routes already exist. Add a Git route (Change). |
| Compatibility report | Competing members of one skill family | Unchanged |
| Canonicalization, digests, provenance, `diff` | Diff as governance | Unchanged |
| `import` | Onboarding existing plugins | Must grow with the supported surface (Add 7) |
| `init` (scaffold generator, CLI only) | Lowers the first-package barrier | Regenerate for v7 |
| Language server and VS Code extension | Authoring tool experience | Update for v7 kinds |
| Self-hosting maintainer definition | Dogfood and drift gate | Regenerate for v7 |
| Browser playground (`web/playground`, `cmd/typeference-wasm`, `cmd/playground-pack`) | The showcase for outside adoption | Rework around v7. Presets: the multi-package Helio example and its marketplace output. Plus an "instantiate a team" demo: a form generated from a context type, a live preview of the generated plugin, and the data file the form would commit. The scaffold generator (ADR-0028) is the precedent. |

## Change

| Area | Change |
| --- | --- |
| Spec version | `schemaVersion: 7`. Cut features are deleted, not deprecated; there is no migrator (same stance as ADR-0030). |
| Pipeline phases | restore, then build, then publish. `link` is removed. Build emits complete artifacts, including `mcp.json`. |
| Tools | The `tool` kind and its schemas are replaced by MCP server documents (Add 2). `requiresTools` becomes a requirement on servers, or on tools a server provides. |
| Field classification rule and the no-untyped-prose rule (ADR-0027) | Replaced by one provenance rule: every emitted byte comes from a file in the package closure, and provenance records which one. Plain prose is allowed wherever it's useful. A model accepts any text a user types, so banning prose blocks nothing; what matters is that every emitted byte is traceable and diffable. `description` stays routing-only, because that is Copilot semantics, not a guarantee. |
| Documents and data | "Context" splits into two concepts. **Documents** are arbitrary Markdown (instructions, guides, references, norms) with no type beyond text. **Data** is typed values that vary per instance. Types describe only data. Rule for authors: if it changes between instances, it's a typed field; if not, it's text. |
| Context types | Data shapes that act as **instantiation contracts**: everyone who instantiates a shared template provides the same type, and a form can be generated from it. Fields keep declaration order and have `type` (`string`, `text`, `boolean`, `integer`, `list<string>`), `required`, `default`, and form metadata: `displayName`, `description` (help text), and `choices` (an enumerated value list). Nothing else until a real case needs it. |
| Package routes | Add a `git` route: a dependency resolves from a repository and tag, and the lockfile records the resolved commit and package digest. Feeds stay for organizations that want them. |
| Slots | Repurposed as the named-parameter mechanism for templates (Add 1), rather than an agent-only feature. |
| Agent emission | Emits the agent frontmatter Copilot supports (Add 3), not only `name` and `description`. |
| Example corpus | Replace `examples/helio` with a fictional example shaped like the motivating suite: an enterprise core library, a team plugin package with template instances, a pipeline-mode skill with an output schema, an MCP server, and a marketplace package. |

## Add

1. **Template parameters and field references.** Any document, skill,
   profile, or agent may declare named, typed parameters. Bodies reference
   fields as `{{param.field}}`, with references only and no expressions.
   - **Profiles are the main template unit.** For example, an
     "onboard team member" profile declares `team: onboarding-team`, and
     instantiating it means supplying one data file of that type.
   - **Forwarding is by name and type.** A profile's parameter binds every
     member skill or document parameter with the same name. Matching names
     with different types is an error. Matching by type alone is never done.
   - **Creating instances.** Use `extends` plus parameter values, or the
     agent-level "instantiate these with my data" shorthand, which generates
     named instances.
   - **Evolving a contract.** Adding a required field to a shared type fails
     the build for every instance that lacks it. The marketplace build lists
     every affected team, which is the intended behaviour. Add fields as
     optional or with a default first.

   Rules:
   - an unknown field is an error;
   - an optional field without a default is an error;
   - only scalar values at first; list rendering is decided later;
   - values are inserted verbatim, with a literal escape for `{{`;
   - only compile-time context can be referenced, never deployment values;
   - templates are never emitted; instances are.

   This also closes the next-steps item "decide how typed context field
   values reach hosts that read only Markdown."

2. **MCP servers as source.** A server kind holding the Agent Plugins 1.0
   transport fields: `stdio` (`command`, `args`, `env`, `cwd`) and
   `streamable-http` (`url`, `headers`). Build emits `mcp.json`. Checks:
   - a plugin that ships a skill also ships every server the skill requires;
   - each server name denotes one configuration across the marketplace;
   - server names must be namespaced, because Copilot gives a plugin's server
     precedence over a user's same-named server;
   - no environment-variable references, because Agent Plugins forbids
     depending on inherited variables and Copilot passes only `PATH`.

   Sign-in stays with the server and Copilot.

3. **The full Copilot plugin surface.**
   - Agent frontmatter: `tools`, `model`, `target`, `user-invocable`,
     `disable-model-invocation`, and agent-scoped `mcp-servers`.
   - Skill frontmatter: `argument-hint`, `user-invocable`,
     `disable-model-invocation`, and `allowed-tools` (explicit opt-in only,
     because it pre-approves tool use for every installer).
   - Hooks, commands, rules, and LSP configuration under `com.github.copilot/`.
   - Inventory the exact supported set from the Copilot CLI plugin reference
     before specifying it.

4. **Agent-only skills as an explicit category.** A skill that depends on its
   agent's context is emitted `user-invocable: false`, or rejected unless
   declared agent-only. Decide whether model invocation outside the agent must
   also be disabled.

5. **Extra files beside skills (required).** Agent Skills bundles
   `references/`, `scripts/`, and `assets/` beside `SKILL.md`, and real
   skills ship reference documents. Today, import refuses them and the spec
   has nowhere to put them. Two routes, chosen by the author:
   - **Plain files.** A skill lists package files under `files`. They are
     emitted verbatim into the skill directory, digested, and tracked by
     provenance. Scripts and assets can only be plain files, and a reference
     document nobody reuses has no reason to be anything else.
   - **Context rendered as a file.** A context the skill holds can render as
     `references/<name>.md` instead of an inline `## Context` section. The
     instructions then point at it, so the model loads it only when needed
     (Agent Skills' progressive disclosure). Choose this route when the text
     is reused across skills, extended, or contains field references.

   Native Markdown authoring is therefore allowed. The value of the typed
   route is reuse, not enforcement.

6. **Output contracts reach the output.** Schemas are validated today but
   never emitted: the generated `SKILL.md` files contain no schema. For
   `pipeline` mode, emit the output schema as a resource file and reference it
   from the instructions, so the runner contract is checked by TypeFerence and
   visible to the model.

7. **Import for the supported surface.** Extend `import` to read everything
   v7 can represent: extra frontmatter, `mcp.json`, hooks, skill resource
   files. That way an existing multi-plugin marketplace repository can be
   imported without `--lossy`.

## Cut

| Area | What goes | Rough size |
| --- | --- | --- |
| `neutral` target | `compile/neutral.go`, neutral digests in every fixture | ~250 lines plus fixture churn |
| ARD and callable publication | `compile/ard.go`, `compile/ard_catalog.go`, `--emit-ard`, 16 fixtures | ~500 lines |
| A2A | `a2a` mode, Agent Card linking, capability `visibility` and exposure | Inside `deploy` and `resolve` |
| Deployment files and `link`, `publish` | `internal/deploy` (keep only the `mcp.json` serializer, moved into build) | ~930 lines |
| Trust metadata and signatures | `internal/trust`, `typeference.trust.tfer`, signature maps, 5 fixtures | ~800 lines |
| Interfaces | `.interface.tfer`, `resolve/interfaces.go`, structural satisfaction | ~100 lines plus fixtures |
| `allowedContextTypes` governance gating | Allow-list intersection | Small; re-add if an organization needs it |
| Modification guards | Skill and binding `sealed`; the extension rule that a sealed base can't be extended | Guards against authors changing behaviour, which they can do anyway by writing a new skill |
| Context type refinement | Context type `embeds`, nominal trust boundaries, field redeclaration and default-conflict rules, `map<T>`, `decimal`, nested named types | Most of the context-type resolver; re-add individual pieces when a real case needs them |
| Schema byte-equality ceremony | Requiring skills that bind a capability to restate identical schemas | Keep schemas; drop the restatement checks |
| Behavioural evaluation and equivalence | `internal/eval`, `eval` and `equivalence` commands, `evals/` | ~1,600 lines |
| Archival languages | Legacy v5 and v3 loaders, 57 archival fixtures (they only reproduce neutral bytes, which v7 no longer emits) | Loader code in `resource` |
| Retired documentation | Whitepaper rewrite; `ard-alignment.md` removed; ADRs 0012/0015-0019/0024 sections superseded by the framing ADR | Docs only |
| Untracked leftovers | Local `src/` and `tests/` (.NET, retired by ADR-0014, untracked) | Local cleanup |

Roughly 4,000-4,500 of about 14,400 non-test Go lines go, along with about
73 of 100 conformance fixtures. The core (grammar, resolver, packages, plugin
emitter, marketplace) is untouched apart from additions. The playground stays
and is reworked.

## Open questions

- **Git route details.** Tags only, or branches too, which would need
  `update` to re-resolve? How do credentials reach a private repository in
  CI? Presumably through Git's own credential helpers, never through source.
- **Form UI output.** The natural design: a form generated from a context
  type writes one `.context.tfer` data file and opens a pull request against
  the team's package, so Git stays the source of truth and the compiler
  validates. Open questions are where the UI is hosted and whether it can
  create a whole new team package or only add instances.
- **Where the "render as file" switch lives.** On the context, on its type as
  a default, or on the skill's reference to it. The reference is the most
  flexible, because the same context could be inline in one skill and a file
  in another.
- **Template syntax.** `{{param.field}}` versus a form the closed grammar
  already lexes, and how a reference renders inside a typed context body.
- **Where template instances are named.** Distinct names (`billing-readme`)
  satisfy the one-name rule. A repository-local output mode would need an
  exception, which is deferred until someone needs it.
- **Model invocation of agent-only skills.** `user-invocable: false` hides the
  slash command, but the model can still choose the skill outside its agent.
- **Copilot-only output.** Which Copilot features ship by default and which
  require opt-in, given that agents are not part of the portable Agent
  Plugins core.

## Suggested landing order

1. Framing ADR plus the v7 specification amendment, folding in the still-
   Proposed ADRs 0029-0034.
2. Cuts, as one change: neutral, ARD, A2A, link and deploy, trust, interfaces,
   eval, playground, archival corpora. Regenerate the remaining fixtures and
   the maintainer distribution.
3. Extra files beside skills (Add 5) and MCP servers as source with the
   build-emitted `mcp.json` (Add 2). These two decide whether existing plugins
   can be represented at all.
4. Template parameters and field references (Add 1), which also unblocks the
   repository-specific skill case.
5. Copilot surface, output contracts, and agent-only skills (Adds 3, 4
   and 6).
6. Import expansion and the replacement example (Add 7 and the example change).
