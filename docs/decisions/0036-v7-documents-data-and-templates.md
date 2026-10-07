# 0036 — Version 7: documents, typed data, and templates

## Status

Proposed (2026-10-06). On acceptance: amends the version 7 specification
("Documents and data", "Context types", "Parameters and field references",
"Instances", "Skill files"). Companion to ADR-0035 and ADR-0037.

## Context

Version 6 typed everything that reached the model. A prose note was a value of
a built-in text type, and context field values were carried only in
`bundle.json`. No `SKILL.md` or agent file ever showed a field value. That
gave authors ceremony without protection, and it left out the one thing typed
values are good for: filling in the parts of a shared skill that differ per
team.

Two cases drive this record:

- **A shared template.** An "onboard team member" profile is instantiated by
  many teams. Every team must provide the same facts (team name, queue,
  repositories). The shape of those facts is a contract that the build
  checks, and that a web form can be generated from.
- **One developer, thirty repositories.** One README skill is 90% identical
  across thirty integration repositories, with team-specific fields in the
  rest.

Real skills also carry files beside `SKILL.md` (reference documents, scripts,
assets), which version 6 had no place for. A pipeline runner validates a
skill's result against a schema, which version 6 validated but never emitted.

## Decision

1. **Documents and data.** A context document is one of two things:
   - a **document**: arbitrary Markdown in its body, with no type;
   - **data**: values of a declared `contextType`, with no body.

   Types describe data only. The rule for authors: if it changes between
   instances, it is a typed field; if it does not, it is text.
2. **Context types are instantiation contracts.** Fields keep declaration
   order. Each field has:
   - a `type` of `string`, `text`, `boolean`, `integer`, or `list<string>`;
   - `required` and `default`;
   - form metadata: `displayName`, `description` (help text), and `choices`
     (allowed string values).

   A context type may name one string field as its `instanceName`. The
   type's metadata and order are sufficient to generate a form whose output
   is a data document.
3. **Parameters.** A document, skill, profile, or agent declares named, typed
   parameters (`parameters`: name to context type). Anything that declares
   parameters is a template:
   - it is never emitted itself;
   - its parameterized members are emitted only as instances.
4. **Field references.** In a template's body, variant instructions, and
   `description`, `{{name.field}}` references a field of a declared
   parameter:
   - only references, never expressions;
   - an undeclared parameter, an unknown field, a `list<string>` field, or an
     optional field without a value or default is an error;
   - values are inserted verbatim;
   - `\{{` writes a literal `{{`;
   - in a document that declares no parameters, `{{` is ordinary text. That
     keeps content such as `${{ secrets.TOKEN }}` safe.
5. **Instances.**
   - **Skill instance.** A skill that `extends` a template skill and supplies
     `with` (parameter name to data document) is a concrete skill. Its name is
     its own identity leaf.
   - **Agent instance.** An agent's `with` binds parameters, by name, for
     every parameterized skill and document the agent composes, including
     those reached through embedded profiles. Each instance of a parameterized
     skill is emitted as `<instance name>-<skill leaf>`, where the instance
     name is the `instanceName` field of the bound data whose type declares
     one. Exactly one bound value must supply it.
   - **Binding rules.** Parameters with one name must have one type across
     everything an agent composes. Every such parameter must be bound, and a
     binding nothing uses is an error. A profile's `parameters` declare its
     contract, and its members must agree with them.
   - **Later.** One agent binding two values to one name is deferred until a
     team needs it.
6. **Contract changes surface everywhere.** Adding a required field to a
   shared context type fails every instance that lacks it. Within a
   marketplace build, that means every affected team, named. That is
   intended. Contracts evolve through optional fields and defaults first.
7. **Rendering held context.**
   - A skill's held context renders inline under `## Context`, as in
     version 6, or, when the skill's reference says `render: file`, as
     `references/<leaf>.md`, which the instructions point at.
   - An agent's held context always renders inline, because agents have no
     resource directory.
   - Data documents never render on their own. Their values reach output
     only through field references.
8. **Skill files.**
   - **Plain files.** A skill's `files` copies package files into its skill
     directory, under `references/`, `scripts/`, or `assets/`. They are
     source members, so they are digested and tracked by provenance. A file
     that is valid UTF-8 is normalized like source text. Any other file is
     copied byte for byte.
   - **Output contracts.** A skill with `inputSchema` or `outputSchema` emits
     them as `references/input.schema.json` and
     `references/output.schema.json` in every artifact. The runner's contract
     and the model's instructions then come from one source.
   - **Independence.** The version 6 independence class and
     `requiresContextTypes` are replaced. A skill shipped directly through a
     plugin's `skills` or `profiles` must have no unbound parameters.

## Consequences

- Authors write Markdown. They declare types only where a value varies, and
  those types do real work: they check every blank in every instance and
  drive generated forms.
- A web form can create a team's instance without knowing any TypeFerence
  rules. It reads a context type, writes a data document, and opens a pull
  request. The compiler validates as usual.
- Instance names are a property of the data. Whoever fills in the form
  chooses the team's prefix, and the name grammar and collision checks catch
  bad choices.
- The open next-steps item about how typed field values reach hosts that
  read only Markdown is closed.

## Alternatives considered

- **Keep prose typed (version 6).** Ceremony with no protection.
- **Untyped string substitution.** It would lose the check that matters at
  scale: thirty instances must each fill every blank correctly.
- **Instance names declared by the agent (`prefix`).** Explicit, but it moves
  a per-team fact out of the team's data and adds a field a form would have
  to fill in anyway.
- **A template engine with conditionals and loops.** That is a second
  language inside prose that the compiler cannot type-check. Variation beyond
  filling blanks belongs in extension or separate skills.
- **Matching parameters by type.** That breaks when two values of one type
  are in play. Names make bindings unambiguous.
