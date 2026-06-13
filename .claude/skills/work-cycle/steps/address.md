Address PR #N per the requested changes (label `needs-work`, you are NOT the most recent pusher).

1. `gh pr view N` + `gh pr diff N` + read ALL review comments. This is your only context — memory lives in GitHub.
2. Work in a worktree on the PR branch. If the branch is behind main → rebase, resolve conflicts, re-run tests.
3. Make changes strictly per the comments. Do NOT expand scope.
4. `make -C services/<svc> test` and `... lint` until green.
5. Resolve the discussed comments, push.
6. Remove `needs-work` (the PR returns to review). Do NOT touch the round counter.
7. Exit.
