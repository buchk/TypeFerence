# 0033 — Import existing GitHub Copilot customizations

## Status

Proposed (2026-09-29). On acceptance: adds `typeference import` to the
specification as an authoring aid outside the build pipeline. Companion to
ADR-0029 and ADR-0030.

## Context

The people version 6 is for already have Copilot customizations: custom agents
in `.github/agents/*.agent.md`, Agent Skills in `.github/skills/*/SKILL.md` (or
`.agents/skills`, `.claude/skills`), and some already have plugins. Asking them
to retype that content into `.tfer` documents is the first thing that would
stop adoption, and a hand conversion is where text silently goes missing.

Those formats carry things version 6 cannot represent: agent frontmatter such
as `tools`, `model`, and `mcp-servers`; files beside a SKILL.md (scripts,
references, assets); and plugin components such as MCP configuration, hooks,
commands, rules, and LSP servers. In version 6, MCP servers are deployment
bindings supplied at link time, and the other components have no source kind.

## Decision

1. **`typeference import <source> --out <dir>`** reads a skill directory, an
   Agent Plugins 1.0 plugin, a Copilot CLI (Claude-format) plugin, or a
   repository, and writes a version 6 package: a manifest, one plugin that
   links every imported agent and skill, one agent document per custom agent
   (description, display name when it differs, body as objectives), and one
   skill document per skill (description, body as instructions). `--name`,
   `--version`, and `--plugin` set identity; defaults are derived from the
   source and documented.
2. **Fail closed, with an explicit escape.** Anything the source contains that
   version 6 cannot represent fails the import, and the error lists every item.
   `--lossy` imports without those items and lists each one it dropped. There
   is no silent best effort.
3. **Never rewrite names.** A skill's declared name must match its directory,
   and every name must satisfy the Agent Skills grammar; a violation is an
   error for the person to fix, because a rewritten name would change the
   slash command people use.
4. **An authoring edge, not a build step.** Import writes only into a new or
   empty directory, performs no network access, and validates what it wrote
   with the ordinary compiler, failing if the result does not validate. Its
   output is ordinary source: from then on the package is maintained as
   version 6, not re-imported.
5. **Repository-wide instructions are not agents.**
   `.github/copilot-instructions.md` is reported and left alone; plugin
   metadata beyond name, description, and version is reported as not carried,
   since build derives `plugin.json`.

## Consequences

- An existing agent or skill reaches a shared marketplace in one command plus
  review, and whatever it could not carry is visible in the command's output.
- Import does not infer composition: every imported agent is a flat agent and
  every skill a root skill. Factoring shared profiles and extensions out of
  the imported set is deliberate authoring work afterward.
- Agents that depend on `tools` or `mcp-servers` import only with `--lossy`;
  their tool access is re-established through tool declarations and link.

## Alternatives considered

- **Best-effort conversion that drops what does not fit.** Rejected: it
  violates fail-closed behavior, and a missing `tools` restriction can change
  what an agent may do.
- **Map `tools` and `mcp-servers` onto tool declarations.** Deferred: Copilot
  tool names are host aliases, not typed contracts, so there is no faithful
  mapping yet.
- **A general v5-to-v6 migrator in the same command.** Rejected: nothing
  outside this repository uses version 5 (ADR-0030 decision 8).
- **Re-importable round trips** (import, edit the Markdown, import again).
  Rejected: two sources of truth; after import the `.tfer` documents are the
  source.
