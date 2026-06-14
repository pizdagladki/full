---
name: review-pr
description: PR review checklist before approval — what to check.
user-invocable: false
---
Check ONLY what affects correctness or violates the issue's criteria (not style for the sake of style):
- whether the acceptance criteria from the linked issue are covered by **table-driven** tests;
- coverage is **≥ 80%** (`make cover`) — a coverage-fail is a blocker;
- edge cases and error handling;
- adherence to the layers (no DB queries in delivery, no business logic in repository; see go-backend-conventions);
- no secrets/keys in the code;
- whether the PR reaches into other services/files out of scope (the "Service/area" and "Out of scope" sections of the issue).
Red CI (lint / test / coverage / build) = an automatic blocker.
Make comments specific, with line references, self-contained — a different agent will resolve them without your context.

## Copilot comments
GitHub Copilot's PR review is **advisory, not binding** — it comments, it never approves, and it never decides merge. For EACH Copilot comment: adjudicate it `apply` (a real correctness / criteria / security defect) or `dismiss` (style-only, false positive, or out of scope) **with a one-line reason**, then ensure its thread is **resolved** (so require-conversation-resolution can't silently block auto-merge). Treat Copilot with caution: a Copilot comment alone is NOT sufficient to fail a PR unless it pinpoints a genuine correctness / criteria / security problem — and never apply it blindly. `apply` findings become your OWN needs-work items, rephrased as actionable comments; `dismiss` findings are resolved-and-explained, not applied.
