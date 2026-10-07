# 0041 — Plugin metadata and carried files

## Status

Proposed (2026-10-07). Amends the version 7 specification ("Identity and
addressing", "Project manifest", "Field tables", "Plugins", "Source
membership and digests", "The agent-plugin target", "Organization
marketplaces", "Import"). Bumps `build.json` to `schemaVersion` 3 and
`bundle.json` to `schemaVersion` 4.

## Context

Feedback from importing an existing Copilot plugin marketplace into
TypeFerence named three things the compiler could not express:

- **Manifest metadata.** The plugins declare `author`, `repository`, and
  `keywords` in `plugin.json`. Build emitted only `$schema`, `name`,
  `version`, and `description`, and import dropped the rest with a note.
- **Files outside skill directories.** The repository ships adoption
  templates beside its plugins, not inside a skill's `references/`,
  `scripts/`, or `assets/`. Version 7 had no place for them, and the plugin
  importer ignored every file it did not read as a component, without a
  note.
- **Repository infrastructure.** The marketplace repository has a README and
  CI workflows. The specification said the published repository "holds only
  compiler output" and that anything else is drift, but gave no way to put
  either into the output.

The feedback asked for explicit output support or an agreed publishing
layer owned in the source repository, rather than silent loss.

The Copilot CLI plugin reference (checked 2026-10-07) lists `author`
(`name`, `email`, `url`), `homepage`, `repository`, `license`, `keywords`, and
`extensions` as optional `plugin.json` members, and the same descriptive
members, plus `category` and `tags`, as optional marketplace entry members.
The vendored Agent Plugins 1.0 schema already permits the `plugin.json`
members.

## Decision

1. **Plugin metadata is plugin source.** A plugin document declares
   `author`, `homepage`, `repository`, `license`, and `keywords`. Build emits
   them unchanged into every artifact's `plugin.json`, after `description`
   and in schema order, and into the plugin's marketplace entry. Each value is
   a non-empty single line; keywords are unique and keep authored order. An
   undeclared member is absent: build never invents one.
2. **Descriptive URLs are source, not deployment.** The identity rule makes
   repository and feed *addresses* deployment because they say where bytes
   are retrieved from. `homepage` and `repository` are shown to people who
   browse a marketplace, nothing retrieves anything from them, and they are
   part of the artifact's bytes. The specification now says so instead of
   leaving the two rules to look contradictory.
3. **Carried files.** A plugin's `files` ships package files at the root of
   each of its artifacts; a manifest's `marketplace.files` ships package files
   at the target root. Both use the skill-file entry grammar (`path`, or
   `{path, as}`), default the destination to the file name, apply the skill
   files' case-collision and directory-spelling rules (ADR-0038, ADR-0039),
   and are source members, so they enter the source digest and packed
   packages.
4. **Build keeps its own paths.** A plugin file may not claim `plugin.json`,
   `mcp.json`, `skills/`, `com.github.copilot/`, `.typeference/`, or `.git/`.
   A marketplace file may not claim `.typeference/`, `.git/`, an artifact
   directory, `.github/plugin/`, or `.github/copilot/`. The last keeps the
   existing rule that build never emits repository settings. CI workflows
   under `.github/workflows/` are allowed: they are files the repository
   runs, not Copilot or repository settings.
5. **Everything published is covered by a digest.** `build.json`
   (`schemaVersion` 3) lists each marketplace file with its destination,
   source, and the SHA-256 of its content as `typeference-directory-v1`
   reads it. Plugin files are already inside an artifact's directory digest,
   and `bundle.json` (`schemaVersion` 4) lists them with source and
   destination, as it lists skill files. The output validator rejects a root
   file that `build.json` does not list or whose digest does not match.
6. **Import carries instead of dropping.** Import writes the source plugin's
   metadata into the plugin document, and every file in a plugin directory
   that is not a component it reads into `plugin-files/` with a plugin
   `files` entry. A manifest member it cannot represent (`extensions` among
   them), a metadata value the rules reject, and a file in a component
   directory (`skills/`, `com.github.copilot/`, a legacy plugin's agent and
   skill directories) that no component reads now fail the import and are
   listed; `--lossy` drops them and lists each one. The source plugin's
   `version` becomes the package version when it is an exact semantic version
   and `--version` is not given; before, it was dropped without a note.
   Import detects a marketplace repository and fails with the plugin
   directories its index lists, instead of reading it as a repository and
   finding nothing.

## Alternatives considered

- **A publishing layer outside build.** The marketplace repository would
  hold build output plus a declared overlay (README, workflows) that the
  source repository owns and drift checks skip. This keeps the compiler
  smaller, but the published repository would no longer be compiler output
  alone, which ADR-0034 made the marketplace's rule ("if you want this you go
  through the compiler"), and the overlay would have no source digest or
  provenance. Rejected.
- **Metadata defaults in the package manifest.** Plugins in one package
  usually share an author, repository, and license, so a manifest default
  with per-plugin override would save repetition. It needs merge rules for
  each member, including whether a plugin can unset an inherited value, and
  it makes a plugin's bytes depend on a second document. Per-plugin fields
  are explicit and can grow defaults later without breaking sources.
  Rejected for now.
- **Generating the README.** Build could render a README from the
  marketplace's plugins. That invents prose the author did not write, and
  teams already have a README they want to keep. Authors can still generate
  one before build. Rejected.
- **`category`, `tags`, and `extensions`.** `category` and `tags` exist only
  on marketplace entries and were not in the feedback. `extensions` is
  client-specific data with no TypeFerence semantics. Both wait for a
  concrete need; import fails closed on `extensions` instead of dropping it.
- **Importing whole marketplaces.** Converting a marketplace repository into
  one marketplace package with one plugin document per plugin would remove
  the last manual step of migration, but it changes import's one-plugin
  shape. Import now fails clearly on a marketplace and lists its plugins.
  Deferred.

## Consequences

- Every committed digest changes once, because `build.json` and
  `bundle.json` change version; the conformance digests, `dist/`, and
  `dist-maintainer/` are regenerated with this change.
- Consumers that check `schemaVersion` must accept `build.json` 3 and
  `bundle.json` 4.
- A marketplace package can now own its whole repository: README, license,
  CI, and per-plugin templates all go through build, so publication still
  replaces the repository's contents and anything else is still drift.
- Fixtures: `016-plugin-metadata`, `017-plugin-files`, and
  `018-marketplace-files` succeed; `149` through `154` reject invalid
  metadata and destinations that claim build's own paths or collide.
