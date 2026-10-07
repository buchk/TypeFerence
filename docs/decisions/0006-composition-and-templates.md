# 0006 — Composition: plugins are the binding root, profiles embed, agents and skills extend

## Status

Accepted (2026-10-07).

## Context

The reuse an organization needs is concrete: a shared core skill that a team
specializes in a few lines; team norms written once and inherited; thirty
team instances of one template, each filling every blank; one installable
plugin that bundles several agents and skills. People also reuse at the
plugin level: a team's plugin taken as is with one skill or one agent line
changed, as someone's personal plugin, still picking up the team's fixes; or
a different plugin built from the core baseline plus a skill pack another
team owns.

Copilot has no way to attach a skill to an agent. Every skill a plugin
installs is available beside every agent it installs, and installed skills
share one namespace keyed by name. The plugin is the unit people install, so
it is the only unit whose contents are real.

## Decision

1. **The plugin is the binding root.** A `.plugin.tfer` decides which agents,
   skills, documents, and native components ship together and binds every
   parameter they declare with `with`. A marketplace lists plugins the way a
   solution lists projects. A plugin with no agents is a skills pack. Each
   mode (`manual`, `pipeline`) is its own artifact.
2. **Plugins and profiles embed.** A plugin embeds any number of profiles and
   plugins; a profile embeds profiles. A profile is a reusable composition
   that may be incomplete (parameters, abstract requirements); embedding a
   plugin takes its composition and data. Bindings, agents, documents, rules,
   commands, hooks, and LSP servers promote through embedding: the
   shallowest wins, identical ones converge, and different ones at one depth
   are ambiguous. Requirements accumulate and never select. Packaging
   (description, metadata, modes, carried files) is never inherited.
   Provenance records every contributor.
3. **Agents are thin and extend additively.** An agent is identity,
   objectives, held documents, Copilot fields, and its own servers; it
   composes nothing. `extends` names one base agent, appends objectives and
   documents, and overlays Copilot fields. An extension takes its base's
   place in a plugin, the way a skill extension rebinds its base's
   capability.
4. **Skills stay `SKILL.md` files and extend additively.** They are never
   inlined into agents, so each remains a slash command and a shared skill
   ships once. `extends` names one base; the extension inherits the base's
   capability and schemas and appends its instructions, per mode; it cannot
   remove base text. A capability groups the skills that implement it, and
   the compatibility report lists plugins that ship competing members.
5. **Documents and typed data.** A context document is free Markdown or the
   values of a context type. Types describe data only and double as form
   definitions (field order, help text, choices, an optional instance-name
   field).
6. **Templates instantiate by name.** Anything that declares `parameters` is
   a template and is emitted only as instances. `{{name.field}}` inserts a
   value; there are no expressions. A skill instance `extends` a template with
   `with`; a plugin's `with` instantiates every parameterized member it
   composes as `<instance name>-<leaf>`. Adding a required field to a shared
   type fails every instance that lacks it, by name.

## Consequences

- A team plugin is a few lines against a shared core profile, and a core fix
  reaches every team on the next build.
- A personal plugin embeds the team's plugin and lists only what it changes:
  an extension of one skill or of the agent. A pin bump and a rebuild bring
  the team's later changes underneath it. Its emitted names match the team
  plugin's, so it is built in its own marketplace and installed instead of
  the team's, not beside it.
- An agent's `## Skills` list and its tool allowlist check cover what the
  artifact actually installs, not a per-agent subset Copilot cannot enforce.
- One plugin binds one value per parameter name, so two agents that need
  different team data ship in two plugins.
- A web form can create a team's instance from a context type without knowing
  any TypeFerence rules.
- A `SKILL.md` is a function of its extension chain, mode, and bound data, so
  one emitted name denotes one body per mode across a build.

## Alternatives considered

- **Plugins link, agents compose.** The previous design: agents embedded
  profiles and bound skills and data, and plugins only listed agents. Agent
  skill lists described an attachment Copilot does not have, a team's plugin
  could not be reused without re-listing its agent, and data bound in an
  agent could not be shared by the skills a plugin ships beside it.
- **Single-base plugin inheritance.** Covers the personal plugin but not a
  plugin assembled from a core baseline and another team's skills; Go-style
  embedding with the existing ambiguity rules covers both.
- **Excluding inherited members.** Not expressible yet; additive extension and
  rebinding cover the observed cases.
- **Override instead of append.** Replacing base text is a new skill or agent,
  not an extension.
- **Untyped string substitution.** It loses the check that matters at scale:
  every instance fills every blank.
- **A template engine with conditionals and loops.** A second language inside
  prose that the compiler cannot check.
- **Typed prose, sealing, interfaces, slots.** Ceremony that blocked nothing
  an author could not route around (ADR-0001).
