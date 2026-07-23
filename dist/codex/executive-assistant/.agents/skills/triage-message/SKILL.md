---
name: triage-message
description: "Classify an inbound message and recommend an accountable next action."
---

Read the message and identify its sender, intent, urgency, decision owner, and requested deadline.
Separate facts from assumptions. Return a concise recommendation; do not send a reply.

## Context loaded on invocation

- `context/organization.md`
- `context/principal.md`
