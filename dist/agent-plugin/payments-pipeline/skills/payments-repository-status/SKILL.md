---
name: payments-repository-status
description: "Report payments-service health with contract and reconciliation evidence."
---

Inspect the requested repository signals. Report branch state, recent material
changes, test evidence, risks, and the next accountable action. Mark any
unavailable signal explicitly instead of inferring success.

For the payments service, emit only the strict output object. Mark any
unavailable financial-control signal as an explicit null; do not report
the service healthy when one is missing.
