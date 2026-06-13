Review PR #N (claimed by you as reviewer; you are NOT the author and NOT the most recent pusher).

1. Delegate to the `code-reviewer` subagent to review the PR diff (it applies the `review-pr` knowledge) in a fresh context.
2. Form a verdict. Red CI = an automatic BAD regardless of the code.
3. GOOD (diff is fine AND CI is green or going green):
   - Approve.
   - Enable GitHub auto-merge (squash) — GitHub merges itself once checks go green.
4. BAD:
   - Leave specific, self-contained comments with line references (a different, fresh agent will resolve them — without your context).
   - Increment the round counter: label `round-(N+1)` instead of `round-N`.
   - Apply `needs-work`. If the count reaches ≥ 3 → apply `needs-human` instead of `needs-work`.
5. Exit.
