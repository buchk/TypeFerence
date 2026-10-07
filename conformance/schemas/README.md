# Output-contract schemas

`tools/validate_output.py` validates built agent-plugin output against these
schemas in CI. See [docs/output-contract.md](../../docs/output-contract.md).

| Directory | Contents | Origin |
| --- | --- | --- |
| `agent-plugins-1.0.0/` | `plugin.schema.json`, `mcp.schema.json` | Vendored unchanged from [agentplugins/agent-plugins-spec](https://github.com/agentplugins/agent-plugins-spec) |
| `copilot/` | `hooks.schema.json`, `lsp.schema.json`, `marketplace.schema.json` | Written by TypeFerence from the Copilot CLI plugin reference |
| `typeference/` | `bundle.schema.json`, `build.schema.json`, `compatibility.schema.json` | TypeFerence's own `.typeference/` files |

## Agent Plugins schemas

The files in `agent-plugins-1.0.0/` are copies of the Agent Plugins 1.0.0
machine-readable schemas, identified by their `$id`
(`https://agent-plugins.org/schemas/1.0.0/...`). They are distributed under the
Apache License, Version 2.0, by the Agent Plugins specification authors. A copy
of the license is at <https://www.apache.org/licenses/LICENSE-2.0>. They have
not been modified. To update them, copy the published files over these and
rerun the output validation.

## Copilot schemas

GitHub does not publish JSON Schemas for Copilot's `hooks.json`, `lsp.json`,
or marketplace index. These record TypeFerence's reading of the Copilot CLI
plugin reference (ADR-0040). They are deliberately strict: they admit what
TypeFerence emits and what the reference documents, and nothing else. When
the reference changes, change the schema and the emitter together.

`marketplace.schema.json` covers only the fields build writes.
