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
