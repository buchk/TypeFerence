# 0004 — Canonicalization rulings for cross-implementation byte identity

**Status:** Accepted (2026-07-08)

## Context

Writing the Go implementation surfaced places where the spec was loose enough that two
honest implementations could produce different bytes from identical source. Under this
project's rules those are spec bugs. Each ruling below changed the spec text
(`docs/specification.md`, "Deterministic compilation" and "Artifact digest algorithm")
and is covered by conformance fixtures.

## Decisions

1. **JSON member order is the specified per-artifact order, not "ordinal order".**
   The old text said "serialize keys in ordinal order", but the shipped `bundle.json`
   keys (`id`, `displayName`, …) have never been alphabetical — they follow the bundle
   shape. The spec now enumerates the canonical member order per artifact and reserves
   sorted-key order for map-like objects (slots, trust manifests, metadata).
   *Alternative rejected:* changing implementations to alphabetical order — that breaks
   every shipped artifact digest for zero semantic gain.

2. **The JSON escape table, indentation, and layout are now normative.** The reference
   implementation's serializer escapes `"` `&` `'` `+` `<` `>` backtick, backslash, all
   control characters, and all non-ASCII (uppercase `\uXXXX`, surrogate pairs), with
   two-space indentation and `": "` separators. Previously this was an implementation
   accident that any second implementation would have missed; it is load-bearing for
   byte identity, so the spec states it. The table was derived by probing the
   reference implementation on .NET 10 and is enforced by `go/internal/jsonx` tests.

3. **Number tokens survive canonicalization byte-for-byte.** Canonical schema strings
   preserve `1.0`, `1e5`, `-0` exactly as authored (the reference implementation
   round-trips raw tokens through its DOM). The spec says so; reformatting numbers is
   non-conforming.

4. **Digest file order is code-point order over forward-slash relative paths.** The old
   algorithm sorted *platform* paths, which makes the digest platform-dependent: `\`
   (0x5C) and `/` (0x2F) order differently against ASCII in between (digits,
   uppercase). A tree containing `foo/bar` next to `fooA` hashes differently on
   Windows and Linux under the old wording. Both implementations now sort the relative
   forward-slash path. Existing published digests are unaffected (the example corpus
   has no such collisions); a conformance fixture pins the behavior.

5. **Canonical string ordering is Unicode code point order, and canonical key spaces
   are ASCII.** The reference implementation's `StringComparer.Ordinal` is UTF-16
   code-unit order, which disagrees with code point order for supplementary-plane
   characters. Rather than demand every implementation reproduce UTF-16 quirks, the
   spec (a) defines canonical order as code point order and (b) restricts the key
   spaces where ordering is observable — slot names and trust metadata keys — to
   ASCII (`[A-Za-z0-9][A-Za-z0-9._-]*`), which both orderings sort identically.
   Resource IDs were already ASCII by grammar. Both implementations enforce the new
   validation. *Alternative rejected:* specifying UTF-16 order — it forces every
   future implementation to emulate a .NET implementation detail forever.

6. **Trust identities must be ASCII.** The reference implementation punycodes IDN
   hosts via `Uri.IdnHost` before publisher-domain alignment; Go's standard library
   does not do IDN. The spec now requires identities to be pre-encoded ASCII
   (punycode), and both implementations reject non-ASCII identities.
   *Alternative rejected:* requiring IDN processing — it drags a Unicode table
   dependency into every implementation for a case publishers can trivially
   pre-encode.

7. **UTF-8 in, UTF-8 out.** Files are UTF-8; a leading BOM is ignored on read;
   artifacts are written without BOM; behavior on invalid UTF-8 is explicitly
   unspecified (implementations may reject). The digest normalizes CRLF to LF and
   strips a leading BOM before hashing.

## Consequences

- `HashDirectory`/`PackageFiles` in the C# implementation now sort by relative
  forward-slash path with a code-point comparer (`CanonicalOrder`); the resource
  loader pins the `.yaml` extension match to be case-sensitive on all platforms.
- New validation errors (non-ASCII slot names, metadata keys, identities) are breaking
  for source trees that used them; given `schemaVersion: 3` is an experimental draft,
  this lands without a schema bump but is called out in the changelog.
