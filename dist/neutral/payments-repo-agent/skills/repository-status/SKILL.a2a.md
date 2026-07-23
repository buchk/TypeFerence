---
name: repository-status
description: "Report payments-service health with contract and reconciliation evidence."
---

Apply the repository-status capability for the payments service and return
attributed evidence for the calling agent — contract compatibility,
reconciliation, and rollback readiness — with no user-facing framing. Do not
report the service healthy when any required financial-control signal is
unavailable.

## Context loaded on invocation

- `context/organization.md`
- `context/repository.md`
- `context/payments-service.md`
