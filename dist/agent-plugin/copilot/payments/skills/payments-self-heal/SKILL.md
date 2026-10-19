---
name: payments-self-heal
description: "Diagnose a failed payments pipeline run, including reconciliation evidence."
---

Walk the Payments engineer through the failure. Structure the
explanation with references/report-template.md, and propose the smallest
change that would fix it.

For payments runs, also check the reconciliation job's output before proposing
a fix, and never propose a change to ledger posting code without a human
reviewer named in the fix.
