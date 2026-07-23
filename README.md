# TypeFerence

**Define organizational agents once. Validate their composition. Compile them into the native shapes your teams already use.**

TypeFerence is an experimental reference implementation of a typed definition and compilation layer for AI agents. It replaces sprawling, duplicated instruction files with Go-like composition: reusable profiles, agent embedding, structurally satisfied interfaces, versioned capabilities, skill implementations, deterministic compilation, provenance, and artifact diffing.

Read the [whitepaper](docs/whitepaper.md), the [rendered PDF](output/pdf/typeference-whitepaper.pdf), the [draft v3 specification](docs/specification.md), or the [ARD alignment notes](docs/ard-alignment.md).

```text
helio/profiles/enterprise-defaults ──embedded by──> helio/profiles/person-defaults     ──embedded by──> helio/executive-assistant
                                  └─embedded by──> helio/profiles/repository-defaults ──embedded by──> helio/payments-repo-agent
```

There is no universal root and no nominal `implements` declaration. Agents embed reusable profiles, promoted behavior is checked for ambiguity, and interfaces are discovered from the resulting slot and capability set.

## Why not just write AGENTS.md?

You can, and for one small agent you often should. TypeFerence becomes useful when those instructions need reuse, review, specialization, provenance, and repeatable output across several agents or hosts.

Agent runtime system prompts are like machine code: they are what the model actually consumes at execution time. `AGENTS.md` and similar host-native instruction files are like assembly language: readable and controllable, but still close to one runtime's concrete shape. TypeFerence is the higher-level language above that. It lets teams model profiles, capabilities, skills, context, and trust metadata once, then compile the result into `AGENTS.md`, Copilot instructions, Cursor rules, neutral bundles, and ARD catalog entries.

## Where it fits

```text
TypeFerence source
    -> native agent artifacts
    -> optional ARD publication and discovery
    -> MCP, A2A, OpenAPI, or host-native invocation
    -> Codex, Copilot, Cursor, Yoke, or another runtime
```

[Agentic Resource Discovery](https://agenticresourcediscovery.org/) helps clients find and verify deployed capabilities. TypeFerence addresses the earlier authoring problem: producing compatible native artifacts from one governed definition. Discovery portability does not itself provide definition portability.

The long-term objective is behavioral equivalence: preserving declared organizational intent across supported hosts closely enough to be measured and governed. V3 provides the common typed source, deterministic adapters, and provenance needed to test that objective; it does not claim that different models or runtimes already behave identically.

The v3 source shape is deliberately small:

```yaml
schemaVersion: 3
kind: agent
id: helio/payments-repo-agent@1.0.0
embeds:
  - helio/profiles/repository-defaults@1.0.0
skills:
  - ref: helio/skills/payments-repository-status@1.0.0
    capability: helio/capabilities/repository-status@1.0.0
```

Use profiles for reusable organizational, domain, or team defaults that should participate in composition without producing their own target bundle.

## Try it in your browser

The **[playground](https://buchk.github.io/TypeFerence/)** runs the real Go
compiler — built for WebAssembly, internals untouched — entirely in your tab.
Edit typed source, watch the compiled Codex/Copilot/Cursor/neutral artifacts,
the embedding graph, and the diagnostics update live. The status-bar digest is
the determinism guarantee made interactive: the Helio example reproduces the
digest of this repository's committed `dist/` exactly, and the self-hosting
example recompiles the repository's own root `AGENTS.md` byte for byte. There
is no backend; nothing you type leaves the browser
([ADR-0010](docs/decisions/0010-browser-playground.md)).

The playground's **Equivalence** tab walks the full BETH loop (ADR-0009)
without ever asking for a credential: it packs scenario × surface cells with
the real `equivalence pack` code, you collect responses by copy/paste from
real hosts (BETH's operator model), it exports the assembled run as a
deterministic `.tar.gz`, you score locally — keys stay in your terminal — and
dropping the resulting `scorecard.json` back onto the page renders adherence,
agreement, and every divergence
([ADR-0011](docs/decisions/0011-playground-live-runs.md)).

## Quick start

Requires Go 1.24+ and nothing else. From clone to compiled artifacts:

```sh
git clone https://github.com/buchk/TypeFerence.git
cd TypeFerence
cd go && go build -o ../bin/ ./cmd/typeference && cd ..

./bin/typeference validate examples/helio
./bin/typeference build examples/helio --target all --out out --emit-ard --publisher-domain helio.example
./bin/typeference diff examples/helio --against dist --emit-ard --publisher-domain helio.example
./bin/typeference inspect helio/payments-repo-agent@1.0.0 --source examples/helio
```

`build` writes the neutral bundle plus Codex, Copilot, and Cursor artifacts under
`out/`. `diff` recompiles and byte-compares against the committed reference output
in `dist/` — "No differences." is the determinism guarantee made visible: your
freshly built compiler reproduces the repository's artifacts exactly.

The binary is fully static (`CGO_ENABLED=0`) with no runtime dependencies. You can
also install it with Go directly (requires the module to be published on the
repository's default branch):

```sh
go install github.com/buchk/TypeFerence/go/cmd/typeference@latest
```

Tagged releases ship prebuilt archives for Linux, macOS, and Windows (amd64/arm64)
with a `SHA256SUMS` file — unpack one binary and put it on `PATH`; there is no
installer to run. The release process is documented in
[docs/release-checklist.md](docs/release-checklist.md).

## Using the CLI

If you downloaded a release archive instead of building from source, unpack
`typeference` (`typeference.exe` on Windows) onto `PATH` and confirm it runs:

```sh
typeference version
```

`<source>` in every command below is a directory of your own typed agent
definitions (`schemaVersion: 3` YAML, shaped like the example above) — clone
this repository to point it at the bundled `examples/helio/` corpus instead.

```text
typeference validate <source> [--trust-config path]
typeference build <source> [--target all|neutral|codex|copilot|cursor] [--out dist]
    [--emit-ard --publisher-domain example.com] [--trust-config path]
    [--trust-signatures signatures.json] [--allow-unsigned-trust]
typeference inspect <agent-id> [--source path]
typeference diff <source> --against <compiled-dir> [--target all]
    [--emit-ard --publisher-domain example.com] [--trust-config path]
    [--trust-signatures signatures.json] [--json] [--allow-unsigned-trust]
typeference eval <source> --scenarios <file-or-dir> [--live] [--model id] [--out dir]
typeference equivalence pack <source> --scenarios <file-or-dir> --out <run-dir>
    [--target all|<name>[,<name>...]]
typeference equivalence score <run-dir> [--live] [--model id]
```

`validate` checks composition and structural typing without writing anything.
`build` compiles to native target artifacts. `diff` recompiles and byte-compares
against an already-built directory — see [Quick start](#quick-start) above for
what "No differences." means. `eval` and `equivalence` are covered in
[Behavioral evals](#behavioral-evals) below; both default to a dry run that
never makes a network call.

## One implementation, one specification

The Go implementation under `go/` is the reference implementation. The
[specification](docs/specification.md) stays normative in principle — the
abstraction is the contract, and anyone may realize it differently — but this
repository's Go compiler is the one living answer ([ADR-0014](docs/decisions/0014-go-only-implementation.md)).
Determinism is preserved and made visible by the
[conformance suite](conformance/README.md): the compiler must reproduce the
committed digests byte-for-byte.

An earlier C# reference implementation was retired when the project committed to
being a tool rather than a multi-implementation standard. Determinism — the
property that makes `diff` and the committed-digest guarantee real — is a property
of one good compiler and was unaffected; only the second implementation, and the
cross-implementation half of the conformance suite, went away. TypeFerence embeds
no LLM provider and holds no deployment state.

## Self-hosting

The agent that maintains this repository is defined in TypeFerence itself, under
[agents/maintainer/](agents/maintainer/). The repository-root
[AGENTS.md](AGENTS.md) and `dist-maintainer/` are compiled artifacts of that
definition; CI recompiles it and fails on any drift. What the exercise revealed
about the type system's limits is recorded honestly in
[ADR-0006](docs/decisions/0006-self-hosting-design-feedback.md).

## Behavioral evals

`typeference eval` runs scenario files (task prompt plus expected-behavior rubric)
against a compiled definition and scores rubric adherence with an LLM judge — dry
run by default, emitting the exact request payloads without any network call. A
pass is an adherence signal, not behavioral equivalence; the framing and its limits
are documented in [evals/README.md](evals/README.md).

`typeference equivalence` (BETH, the Behavioral Equivalence Test Harness) is the
deployment-side counterpart: `pack` lays out the same scenarios as run-ready cells
per compiled target surface, an operator collects one response per cell from a real
host, and `score` reports adherence per surface and agreement across surfaces,
listing every divergence. A scorecard is one observation per surface, not a proof;
see [ADR-0009](docs/decisions/0009-behavioral-equivalence-harness.md).

`--emit-ard` publishes one canonical TypeFerence source-package entry and one precompiled bundle entry for each concrete agent and selected target. Compiled entries carry `derivedFrom` provenance back to the canonical source digest. ARD is an envelope: a target-aware installer must understand static Codex, Copilot, or Cursor bundles, while callable MCP or A2A resources require their own deployed endpoint and native card.

### Trust manifests

An optional `typeference.trust.yaml` at the source root enriches the draft AI Catalog `trustManifest` for the source package and compiled bundles. It can declare DID, SPIFFE, or HTTPS identity; trust schemas; attestation and provenance references; policy or enterprise verification metadata; and an intent to sign. TypeFerence validates and publishes these declarations without resolving remote documents or asserting that an external authority has verified them.

```yaml
schemaVersion: 1
source:
  identity: did:web:helio.example:typeference:source:helio
  identityType: did
  attestations:
    - type: https://slsa.dev/provenance/v1
      uri: https://trust.helio.example/provenance/source.intoto.jsonl
      digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
bundles:
  identityTemplate: spiffe://helio.example/typeference/{target}/{agent}
  identityType: spiffe
  metadata:
    com.helio.governance:
      policyDigest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      runtimeEvidenceProfile: tag:agentrust.io,2026:trace-v0.1
  signatureIntent:
    algorithm: ES256
    keyRef: did:web:helio.example#catalog-signing
```

TypeFerence does not hold signing keys. An external signer can produce detached JWS values over the unsigned, JCS-canonicalized trust manifests and provide them through `--trust-signatures signatures.json`. Use `--allow-unsigned-trust` only to emit signing input when `signatureIntent.required` is true; normal publication fails closed without those signatures. The signature map must be outside the source root so adding signatures cannot change the source digest they sign. Identical source, trust config, and signature map inputs produce byte-identical output.

## Repository map

- `go/` - the Go implementation: compiler, target adapters, CLI, language server (`cmd/typeference-lsp`), and eval harness.
- `conformance/` - shared cross-implementation conformance fixtures (byte-identity contract).
- `examples/helio/` - fictional cross-domain organization.
- `web/playground/` - browser playground: the Go compiler built for `js/wasm`, plus the BETH operator console.
- `agents/maintainer/` - this repository's maintainer agent, defined in TypeFerence.
- `evals/` - behavioral eval scenarios and honest framing.
- `docs/specification.md` - normative v3 behavior.
- `docs/decisions/` - architecture decision records.
- `docs/whitepaper.md` and `output/pdf/typeference-whitepaper.pdf` - design paper.
- `CHANGELOG.md` and `docs/release-checklist.md` - versioning and release process.

## Design boundaries

- Agents may embed multiple profiles or agents; profiles may embed other profiles; local slots and capability bindings resolve promoted-name ambiguity.
- Interfaces may embed interfaces and are satisfied structurally, without declarations on agents.
- Capabilities are explicit, versioned method slots; skills are concrete implementations that bind those capabilities.
- Context is referenced and loaded only with the skill that needs it.
- Target adapters emit platform-native shapes while retaining the portable fields each target supports.
- Build output is deterministic and carries provenance.
- No deployment state, hosted runtime, or model credentials in v3.
- No ARD registry lifecycle, federation, dependency, install-safety, or deployment metadata in core TypeFerence semantics.
- Structural validation does not guarantee identical LLM behavior across models or hosts.
- ARD publication wraps selected target outputs; it is not itself a compilation target or execution runtime.

The current adapters are reference implementations against fast-moving platform formats. Review generated artifacts before production use.

TypeFerence is licensed under Apache-2.0. Helio Works is fictional.
