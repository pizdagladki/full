Review PR #N (claimed by you as reviewer; you are NOT the author and NOT the most recent pusher).

1. **Sync first:** `git fetch origin` and confirm you are reviewing the **latest pushed head** (`gh pr view N --json headRefOid`; if it changed since you claimed, re-read). Then gather context and delegate: run `gh pr view N` (PR metadata + the linked issue) and `gh pr diff N` (the current-head diff), and check CI status. Read the linked issue's acceptance criteria. Then delegate the diff + PR metadata + the acceptance criteria to BOTH the `code-reviewer` and the `security-reviewer` subagents (fresh context). They review what you pass in — they do NOT fetch the diff themselves.
2. Form a verdict from both reviews. Red CI = an automatic BAD regardless of the code.
3. GOOD (diff is fine AND CI is green or going green):
   - Approve.
   - Enable GitHub auto-merge (squash) — GitHub merges itself once checks go green.
4. BAD:
   - Leave specific, self-contained comments with line references (a different, fresh agent will resolve them — without your context).
   - Round counter: if the PR has no `round-*` label → add `round-1`; otherwise replace `round-N` with `round-(N+1)`.
   - Apply `needs-work`. If the round count reaches ≥ 3 → apply `needs-human` instead of `needs-work`.
   - RELEASE your reviewer claim: unassign / un-request yourself so the PR returns to the unclaimed "awaiting review" pool — a different fresh agent re-reviews after the changes.
5. Exit.
