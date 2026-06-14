Address PR #N per the requested changes (label `needs-work`, you are NOT the most recent pusher). Print `[ADDRESS] #<N> start`.

1. Fetch context via MCP: `mcp__github__get_pull_request` (#N) + `mcp__github__get_pull_request_files` (#N, the diff) + `mcp__github__get_pull_request_comments` (#N — read ALL review comments). This is your only context — memory lives in GitHub. These are the **reviewer's own** comments — they already fold in any applied Copilot findings, and the reviewer already adjudicated + resolved Copilot's threads, so do NOT separately chase Copilot threads. Any new Copilot comments on your push are adjudicated in the NEXT review round.
2. **Sync first:** `git fetch origin`. Work in a worktree on the PR branch at its **latest remote head** — another agent may have pushed since the review: `git worktree add ../<branch> <branch>` then `git -C ../<branch> reset --hard origin/<branch>`. If it is behind `origin/main` → rebase onto `origin/main`, resolve conflicts, re-run tests.
3. Make changes strictly per the comments. Do NOT expand scope.
4. `make -C services/<svc> test`, `... cover` (≥80%) and `... lint` until green. Fix any spelling issue the local `typos` hook or the CI `spell` job reports (or add a legitimate domain term to `_typos.toml` at the repo root) before updating the PR.
5. Resolve the discussed comments, push (Bash).
6. Remove `needs-work` — `mcp__github__update_issue` on #N with the new label set (the PR returns to review). Do NOT touch the round counter. (The board is automated — do not touch it.) Print `[ADDRESS] #<N> pushed, back-to-review`.
7. Exit.
