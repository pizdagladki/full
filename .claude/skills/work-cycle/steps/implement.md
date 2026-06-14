Implement issue #N (already claimed by you — you are the assignee). Print `[IMPLEMENT] #<N> start`.

> Board status is automated by the project's GitHub workflows (issue assigned → In Progress, PR merged → Done). Do NOT touch the Projects board from here — claiming the issue (the assignment done in `select.md`) is what moves the card.

GitHub issue/PR operations use the **GitHub MCP** tools (`mcp__github__*`, owner `pizdagladki`, repo `full`); local code + git via Bash.

1. `mcp__github__get_issue` (#N). If the acceptance criteria are unverifiable/unclear → apply `needs-human` (`mcp__github__update_issue`), exit. Do NOT guess.
2. **Sync first:** `git fetch origin`. Work in a separate git worktree, branching `feat/N-<slug>` off the freshly-fetched `origin/main` (NOT a stale local `main`): `git worktree add ../<slug> -b feat/N-<slug> origin/main`.
3. Full autonomy: for a non-trivial task, enter plan mode yourself, draft a plan, and implement immediately (WITHOUT human approval). Trivial — straight to code.
4. Code strictly within the zone from the "Service/area" section. Follow `go-backend-conventions` (it loads itself). New service/resource → the `new-service` / `new-resource` skills.
5. Write **table-driven** tests for EVERY acceptance criterion; generate mocks for interfaces with mockgen (`make mocks`).
6. `make -C services/<svc> test`, `... cover` (≥80%) and `... lint` until green; print `[IMPLEMENT] #<N> green`. Show the output. Fix any spelling issue the local `typos` hook or the CI `spell` job reports (or add a legitimate domain term to `_typos.toml` at the repo root) before opening the PR.
7. SELF-REVIEW: run `git diff main...HEAD` (Bash) to capture your diff, then delegate that diff + the issue's acceptance criteria to BOTH the `code-reviewer` and the `security-reviewer` subagents (fresh context; they review what you pass in — they do NOT fetch it). Fix what they find; print `[IMPLEMENT] #<N> self-review done`.
8. Open the PR via `mcp__github__create_pull_request` (base `main`, head `feat/N-<slug>`): body with `Closes #N` + the attached test/lint output, **filling the PR template AND checking its boxes** (`make lint` green, service tests green, check output attached) and **listing the affected services** — never leave the Checks section empty. Print `[PR] opened #<N> for issue #<N>` (PR number then issue number).
9. Exit. Do NOT merge — the reviewer does that.
