# Trust model invariants

The trust model is declarative supply-chain metadata, specified in
`docs/specification.md` ("Trust metadata compilation" and "Trust Manifest
signatures"). The invariants that must survive every change:

- **The signature map lives outside the source root.** The source-package digest
  covers every file under the source root; a signature stored inside would change
  the digest embedded in the very payload being signed. Both implementations reject
  a signature map path beneath the source root.
- **Fail-closed stays fail-closed.** When `signatureIntent.required` is true and no
  signature is imported, publication fails. `--allow-unsigned-trust` exists solely
  to emit the payload for an external signer, and its output is visibly unsigned.
- **TypeFerence never signs.** It imports externally produced compact detached JWS
  strings, validates their shape, and never verifies cryptographic validity,
  resolves keys, or dereferences any trust URI.
- **Identities are ASCII** (punycode for internationalized authorities) so publisher
  alignment does not depend on implementation-specific IDN handling.

Changes that touch any of these need a specification amendment, an ADR, and
conformance fixtures before implementation.
