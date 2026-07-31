# 0022 — Separate build, link, and source identity

**Status:** Accepted (2026-07-29)

## Context

The project manifest briefly accepted `deployment.a2aBaseUrl` and
`deployment.mcpCommand`. The Codex target then used the command to change generated
configuration bytes. This violated the existing rule that deployment metadata
does not participate in target compilation, and it made an environment endpoint
part of the source package digest. Hashing the whole source directory after
writing output compounded the problem by allowing generated artifacts to affect
source identity.

A bare token such as `${TYPEFERENCE_MCP_COMMAND}` would only move the ambiguity:
Codex does not define that token as a portable command substitution mechanism, and
an active config containing it is not a complete artifact.

## Decision

Identity is source; address is deployment.

The project manifest advances to schema version 2 and contains only stable package
identity, publisher identity, and exact source dependencies. Build produces
deterministic unlinked target packages plus typed link requirements. It emits no
active host configuration that depends on commands, endpoints, environment
variables, or credentials.

Deployment lives in an explicit external closed-schema file. Target adapters have
two phases:

```text
Build(resolved definition) -> deterministic unlinked package
Materialize(unlinked package, deployment bindings) -> runnable target
```

The linker validates selected modes, tool/provider bindings, endpoint syntax, and
environment/secret references, then uses structural serializers for host-native
configuration. It never changes the compiled semantics.

Source identity hashes an explicit resource set and lockfile. It excludes output,
restored packages, deployment/feed configuration, signature maps, VCS data, and
unreferenced files. Target provenance records source and dependency digests.
Linked provenance records the deployment digest separately.

## Consequences

- Staging and production share source and unlinked-target digests.
- Codex, Copilot, Cursor, MCP, A2A, and future adapters can share one typed linker
  boundary without pretending their runtime configuration formats are source.
- Callable cards are emitted only when an endpoint/provider is explicitly linked.
- Token replacement is available only as an explicit deployment-provider
  projection; it is never implicit string substitution inside compiled files.

## Alternatives considered

- **Keep deployment in `typeference.yaml`.** Changes source identity for an
  environmental address. Rejected.
- **Bake CLI endpoint flags into build output.** Preserves manifest purity but
  still makes build an environment-specific transform. Rejected.
- **Emit unresolved tokens in active config.** Neither typed nor guaranteed to be
  interpreted by the host. Rejected.
