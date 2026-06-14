Pick EXACTLY ONE unit of work. Print `[SELECT] scanning`. GitHub reads/writes use the **GitHub MCP** tools (`mcp__github__*`, owner `pizdagladki`, repo `full`).

0. KILL-SWITCH: `mcp__github__list_issues` (labels=["fleet-stop"], state=open). If any open issue is labeled `fleet-stop` → print `[SELECT] fleet-stop` then `WORK_QUEUE_EMPTY` and exit. The fleet has been stopped by a human.

Priority — take the FIRST matching one, in this order (finishing beats starting):

1. CHANGES. A PR labeled `needs-work` where you are NOT the most recent pusher.
   - Find: `mcp__github__list_pull_requests` (state=open); for candidates read labels/commits via `mcp__github__get_pull_request`. Exclude any PR whose most-recent pusher is you.
   - If the round count (label `round-N`) is ≥ 3 → apply `needs-human`, remove `needs-work` (`mcp__github__update_issue` on the PR number with the new label set), skip this PR.
   - Otherwise → `[PICKED] type=address #<N>`; work type = address, PR = #N.

2. REVIEW. An open PR you are NOT the author of and NOT the most recent pusher, with no assignee.
   - Find: `mcp__github__list_pull_requests` (state=open); details via `mcp__github__get_pull_request`.
   - Claim: assign yourself — `mcp__github__update_issue` on the PR number, assignees=[you] → wait a random 1–3 s → re-read (`mcp__github__get_pull_request`). If someone else claimed it before you → yield, return to the start of selection.
   - Otherwise → `[PICKED] type=review #<N>`; work type = review, PR = #N.

3. NEW ISSUE. An open issue labeled BOTH `task` AND `owner-agreed`, with no assignee, whose blockers are ALL closed. An issue WITHOUT `owner-agreed` is ignored entirely — the owner has not signed off on it yet.
   - Find: `mcp__github__list_issues` (labels=["task","owner-agreed"], state=open).
   - Blockers: parse `Depends on #X` / `Blocked by #X` from the body. For each, `mcp__github__get_issue` → if any is open, skip this issue.
   - Claim: assign yourself (`mcp__github__update_issue`, assignees=[you]) → wait 1–3 s → re-read (`mcp__github__get_issue`). If the assignee isn't you → yield, return to the start of selection. (Assigning yourself fires the `project-in-progress` automation that moves the card to In Progress — do NOT touch the board manually.)
   - Otherwise → `[PICKED] type=implement #<N>`; work type = implement, issue = #N.

DEADLOCK CIRCUIT BREAKER: if open `task`+`owner-agreed` issues exist but none qualify (all with open blockers / a suspected A↔B cycle), and there's no changes/review work → apply `needs-human` to the oldest blocked issue (`mcp__github__update_issue`), print `[SELECT] deadlock -> needs-human #<N>` then `WORK_QUEUE_EMPTY`, exit.

If nothing was found → print `[SELECT] empty` then `WORK_QUEUE_EMPTY`, exit.
