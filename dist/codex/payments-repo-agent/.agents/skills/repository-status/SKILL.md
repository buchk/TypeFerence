---
name: repository-status
description: "Report payments-service health with contract and reconciliation evidence."
---

Apply the repository-status capability for the payments service and emit only
the strict output object. Mark any unavailable financial-control signal as an
explicit null; do not report the service healthy when one is missing.

## Context loaded on invocation

- `context/organization.md`
- `context/repository.md`
- `context/payments-service.md`
