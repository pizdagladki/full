Implement issue #N (already claimed by you — you are the assignee).

> Board status is automated by the project's GitHub workflows (issue assigned → In Progress, PR merged → Done). Do NOT touch the Projects board from here — claiming the issue (the assignment done in `select.md`) is what moves the card.

1. `gh issue view N`. If the acceptance criteria are unverifiable/unclear → apply `needs-human`, exit. Do NOT guess.
2. **Sync first:** `git fetch origin`. Work in a separate git worktree, branching `feat/N-<slug>` off the freshly-fetched `origin/main` (NOT a stale local `main`): `git worktree add ../<slug> -b feat/N-<slug> origin/main`.
3. Full autonomy: for a non-trivial task, enter plan mode yourself, draft a plan, and implement immediately (WITHOUT human approval). Trivial — straight to code.
4. Code strictly within the zone from the "Service/area" section. Follow `go-backend-conventions` (it loads itself). New service/resource → the `new-service` / `new-resource` skills.
5. Write **table-driven** tests for EVERY acceptance criterion; generate mocks for interfaces with mockgen (`make mocks`).
6. `make -C services/<svc> test`, `... cover` (≥80%) and `... lint` until green. Show the output.
7. SELF-REVIEW: run `git diff main...HEAD` to capture your diff, then delegate that diff + the issue's acceptance criteria to BOTH the `code-reviewer` and the `security-reviewer` subagents (fresh context; they review what you pass in — they do NOT fetch it). Fix what they find.
8. Open a PR: body with `Closes #N`, attach the test/lint output, fill in the PR template.
9. Exit. Do NOT merge — the reviewer does that.
