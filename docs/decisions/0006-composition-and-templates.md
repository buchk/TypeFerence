# 0006 — Composition: plugins link, profiles embed, skills extend, templates instantiate

## Status

Accepted (2026-10-07).

## Context

The reuse an organization needs is concrete: a shared core skill that a team
specializes in a few lines; team norms written once and inherited; thirty
team instances of one template, each filling every blank; one installable
plugin that bundles several agents and skills. Copilot itself has no way to
attach skills to an agent, and installed skills share one namespace keyed by
name.

## Decision

1. **Plugins link; they do not compose.** A `.plugin.tfer` lists agents,
   profiles, skills, and native components that ship together, like a
   solution file. A plugin with no agents is a skills pack. Each mode
   (`manual`, `pipeline`) is its own artifact.
2. **Skills stay `SKILL.md` files.** They are never inlined into agents, so
   each remains a slash command and a shared skill ships once.
3. **Profiles and agents embed.** Bindings, documents, rules, commands, and
   hooks promote through embedding; the shallowest binding wins, identical
   ones converge, and different ones at one depth are ambiguous. Requirements
   accumulate and never select. Agents stay thin: identity, objectives, and
   composition. Provenance records every contributor.
4. **Skills extend additively.** `extends` names one base. The extension
   inherits the base's capability and schemas and appends its instructions,
   per mode; it cannot remove base text. A capability groups the skills that
   implement it, and the compatibility report lists plugins that ship
   competing members.
5. **Documents and typed data.** A context document is free Markdown or the
   values of a context type. Types describe data only and double as form
   definitions (field order, help text, choices, an optional instance-name
   field).
6. **Templates instantiate by name.** Anything that declares `parameters` is
   a template and is emitted only as instances. `{{name.field}}` inserts a
   value; there are no expressions. A skill instance `extends` a template with
   `with`; an agent's `with` instantiates every parameterized member it
   composes as `<instance name>-<leaf>`. Adding a required field to a shared
   type fails every instance that lacks it, by name.

## Consequences

- A team specialization is a few lines against a shared core, and a core fix
  reaches every team on the next build.
- A web form can create a team's instance from a context type without knowing
  any TypeFerence rules.
- A `SKILL.md` is a function of its extension chain, mode, and bound data, so
  one emitted name denotes one body per mode across a build.

## Alternatives considered

- **Override instead of append.** Replacing base text is a new skill, not an
  extension.
- **Untyped string substitution.** It loses the check that matters at scale:
  every instance fills every blank.
- **A template engine with conditionals and loops.** A second language inside
  prose that the compiler cannot check.
- **Typed prose, sealing, interfaces, slots.** Ceremony that blocked nothing
  an author could not route around (ADR-0001).
