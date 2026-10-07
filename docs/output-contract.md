# Output contract

This document lists every file `typeference build` emits for the
`agent-plugin` target, which format governs each one, and how CI checks it.
The specification ("The agent-plugin target" and "Native Copilot components")
is normative for the exact bytes. This document is the consumer's view: what a
marketplace, an installer, or a tool reading build output may rely on.

## Files

Paths are relative to `<out>/agent-plugin/`. `<artifact>` is a plugin
directory: `<plugin>` for manual mode, `<plugin>-pipeline` for pipeline mode.

| Path | Format owner | Emitted when | Checked against |
| --- | --- | --- | --- |
| `.github/plugin/marketplace.json` | Copilot | the manifest declares a marketplace | `conformance/schemas/copilot/marketplace.schema.json` |
| `.typeference/build.json` | TypeFerence (`schemaVersion` 3) | always | `conformance/schemas/typeference/build.schema.json` |
| `.typeference/compatibility.json` | TypeFerence (`schemaVersion` 1) | always | `conformance/schemas/typeference/compatibility.schema.json` |
| `<marketplace file>` | the author's | the manifest's `marketplace.files` lists it | its digest in `build.json` |
| `<artifact>/plugin.json` | Agent Plugins 1.0 | always | `conformance/schemas/agent-plugins-1.0.0/plugin.schema.json` |
| `<artifact>/mcp.json` | Agent Plugins 1.0 | a shipped skill requires a server in this mode | `conformance/schemas/agent-plugins-1.0.0/mcp.schema.json` |
| `<artifact>/skills/<skill>/SKILL.md` | Agent Skills, plus Copilot fields | per shipped skill and instance | frontmatter fields, name grammar, name equals directory, length limits |
| `<artifact>/skills/<skill>/references/input.schema.json`, `output.schema.json` | JSON Schema | the skill declares input or output | a valid JSON Schema |
| `<artifact>/skills/<skill>/{references,scripts,assets}/...` | the author's | the skill lists files or file documents | digest only |
| `<artifact>/com.github.copilot/agents/<agent>.agent.md` | Copilot | per linked agent | frontmatter fields, name equals file name, `mcp-servers` shape, no `${PLUGIN_DATA}` |
| `<artifact>/com.github.copilot/rules/<rule>.md` | Copilot | per shipped rule | optional frontmatter of `paths` and `description`, non-empty body |
| `<artifact>/com.github.copilot/commands/<command>.md` | Copilot | per shipped command | frontmatter fields, required `description`, non-empty body |
| `<artifact>/com.github.copilot/hooks/hooks.json` | Copilot | the artifact ships a hook | `conformance/schemas/copilot/hooks.schema.json` |
| `<artifact>/com.github.copilot/lsp.json` | Copilot | the artifact ships an LSP server | `conformance/schemas/copilot/lsp.schema.json` |
| `<artifact>/<plugin file>` | the author's | the plugin's `files` lists it | digest only |
| `<artifact>/.typeference/bundle.json` | TypeFerence (`schemaVersion` 4) | always | `conformance/schemas/typeference/bundle.schema.json` |

Nothing else is emitted. In particular, build never writes repository or
enterprise settings (no carried file may land beneath `.github/copilot/`), and
it writes no file under `com.github.copilot/` other than those listed.
`plugin.json` and each marketplace entry carry the plugin's `author`,
`homepage`, `repository`, `license`, and `keywords` only when the plugin
declares them (ADR-0041).

## TypeFerence's own files

The `.typeference/` files are TypeFerence's provenance and integrity record.
Copilot ignores them.

- `build.json` lists every artifact directory with its plugin identity, mode,
  owning package source digest, and `typeference-directory-v1` digest. A
  consumer can recompute an artifact's digest to confirm it was not changed
  after build. It also lists each marketplace file with its destination,
  source, and file digest, so every file in the target is either an index
  file or covered by a digest; the validator rejects any other root file.
- `compatibility.json` lists capabilities that two or more distinct skill
  implementations provide in one mode, so a marketplace can warn before
  installing plugins that compete for the same requests.
- `bundle.json` records what one artifact ships and why: resolved agents with
  every contributor, skills with their template and bound data, servers,
  rules, commands, hooks, LSP servers, and the plugin's carried files. Identities have the form
  `<package>/<path>@<version>`. `templateId` is empty for a skill, rule, or
  command that is not an instance.

**Versioning.** Each file carries `schemaVersion`. Adding, removing, renaming,
or retyping a field increments it, and the matching schema under
`conformance/schemas/typeference/` changes in the same commit. The schemas
reject unknown fields, so a field added without a version bump fails CI.
A consumer should refuse a `schemaVersion` it does not know.

## Host formats

The Agent Plugins schemas are vendored unchanged. Copilot publishes no schema
for `hooks.json`, `lsp.json`, or its marketplace index, so TypeFerence's
schemas for them record its reading of the Copilot CLI plugin reference
(ADR-0040). They are strict on purpose. If the reference changes, the emitter,
the compile-time checks, and the schema change together.

Frontmatter is checked by field list rather than by schema, because hosts read
it as YAML and accept only fields they recognize:

| Component | Allowed fields |
| --- | --- |
| skill | `name`, `description`, `license`, `compatibility`, `metadata`, `allowed-tools`, `argument-hint`, `user-invocable`, `disable-model-invocation` |
| agent | `name`, `description`, `model`, `tools`, `user-invocable`, `disable-model-invocation`, `mcp-servers` |
| command | `description`, `argument-hint`, `allowed-tools`, `disable-model-invocation` |
| rule | `paths`, `description` |

## How it is checked

```bash
pip install -r tools/requirements-output.txt
python tools/validate_output.py <build output directory> ...
```

CI's `output-contract` job builds every success conformance fixture (setting
`TF_OUTPUT_DIR` so the conformance runner keeps each output), the Helio
reference, and the maintainer plugin, then validates all of them, including
the committed `dist/` and `dist-maintainer/`.

What this does not prove: that Copilot loads the output the way the reference
says. That still has to be confirmed in Copilot itself; ADR-0040 lists the
open questions.
