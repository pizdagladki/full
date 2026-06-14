Address PR #N per the requested changes (label `needs-work`, you are NOT the most recent pusher).

1. `gh pr view N` + `gh pr diff N` + read ALL review comments. This is your only context — memory lives in GitHub.
2. **Sync first:** `git fetch origin`. Work in a worktree on the PR branch at its **latest remote head** — another agent may have pushed since the review: `git worktree add ../<branch> <branch>` then `git -C ../<branch> reset --hard origin/<branch>`. If it is behind `origin/main` → rebase onto `origin/main`, resolve conflicts, re-run tests.
3. Make changes strictly per the comments. Do NOT expand scope.
4. `make -C services/<svc> test`, `... cover` (≥80%) and `... lint` until green.
5. Resolve the discussed comments, push.
6. Remove `needs-work` (the PR returns to review). Do NOT touch the round counter.
7. Exit.
