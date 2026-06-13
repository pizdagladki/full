Implement issue #N (already claimed by you).

1. `gh issue view N`. If the acceptance criteria are unverifiable/unclear → apply `needs-human`, exit. Do NOT guess.
2. Move the Projects card to In Progress.
3. Work in a separate git worktree. Branch `feat/N-<slug>` off a fresh main.
4. Full autonomy: for a non-trivial task, enter plan mode yourself, draft a plan, and implement immediately (WITHOUT human approval). Trivial — straight to code.
5. Code strictly within the zone from the "Service/area" section. Follow `go-backend-conventions` (it loads itself). New service/resource → the `new-service` / `new-resource` skills.
6. Write tests for EVERY acceptance criterion.
7. `make -C services/<svc> test` and `... lint` until green. Show the output.
8. SELF-REVIEW: delegate to the `code-reviewer` subagent to review your diff (fresh context) against the criteria, conventions, and security. Fix what it finds.
9. Open a PR: body with `Closes #N`, attach the test/lint output, fill in the PR template.
10. Exit. Do NOT merge — the reviewer does that.
