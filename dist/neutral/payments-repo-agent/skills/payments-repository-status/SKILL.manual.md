---
name: payments-repository-status
description: "Report payments-service health with contract and reconciliation evidence."
---

Inspect the requested repository signals. Report branch state, recent material
changes, test evidence, risks, and the next accountable action. Mark any
unavailable signal explicitly instead of inferring success.

For the payments service, also report payment-contract compatibility,
reconciliation checks, and rollback readiness, and surface uncertainty
conversationally. Do not report the service healthy when any required
financial-control signal is unavailable.
