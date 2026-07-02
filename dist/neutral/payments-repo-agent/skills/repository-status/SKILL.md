---
name: repository-status
description: "Report payments-service health with contract and reconciliation evidence."
---

Apply the repository-status contract, then include payment-contract compatibility, reconciliation checks, and rollback readiness.
Do not report the service healthy when any required financial-control signal is unavailable.

## Context loaded on invocation

- `context/organization.md`
- `context/safety-policy.md`
- `context/repository.md`
- `context/payments-service.md`
