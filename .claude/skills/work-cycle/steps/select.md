Pick EXACTLY ONE unit of work. Tool — `gh`.

0. KILL-SWITCH: if the repo has an open issue labeled `fleet-stop` → print `WORK_QUEUE_EMPTY` and exit. The fleet has been stopped by a human.

Priority — take the FIRST matching one, in this order (finishing beats starting):

1. CHANGES. A PR labeled `needs-work` where you are NOT the most recent pusher.
   - If the number of revision rounds (label `round-N`) is ≥ 3 → apply `needs-human`, remove `needs-work`, skip this PR.
   - Otherwise → work type = address, PR = #N.

2. REVIEW. An open PR in "awaiting review" state where you are NOT the author and NOT the most recent pusher, with no reviewer assigned.
   - Claim: assign yourself as reviewer → wait a random 1–3 s → re-read the PR. If a reviewer claimed it before you → yield, return to the start of selection.
   - Otherwise → work type = review, PR = #N.

3. NEW ISSUE. An open issue labeled `task`, with no assignee, whose blockers are ALL closed.
   - Blockers: parse `Depends on #X` / `Blocked by #X` from the body. For each, `gh issue view X --json state`. If any is open → skip this issue.
   - Claim: assign yourself → wait 1–3 s → re-read. If the assignee isn't you → yield, return to the start of selection.
   - Otherwise → work type = implement, issue = #N.

DEADLOCK CIRCUIT BREAKER: if open `task` issues exist but none qualify (all with open blockers / a suspected A↔B cycle), and there's no changes/review work → apply `needs-human` to the oldest blocked issue, print `WORK_QUEUE_EMPTY`, exit.

If nothing was found → print `WORK_QUEUE_EMPTY`, exit.
