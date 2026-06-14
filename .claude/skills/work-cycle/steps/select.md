Pick EXACTLY ONE unit of work. Print `[SELECT] scanning`. GitHub reads/writes use the **GitHub MCP** tools (`mcp__github__*`, owner `pizdagladki`, repo `full`).

0. KILL-SWITCH: `mcp__github__list_issues` (labels=["fleet-stop"], state=open). If any open issue is labeled `fleet-stop` → print `[SELECT] fleet-stop` then `WORK_QUEUE_EMPTY` and exit. The fleet has been stopped by a human.

CLAIMING IS A LEASE, NOT A LOCK — read before the cascade:
- **Exclusive claim.** To claim a PR/issue: `mcp__github__update_issue` assignees=[you] AND post a stamp `mcp__github__add_issue_comment` (#N) `🤖 [CLAIM] <FLEET_AGENT> <UTC-ISO8601>` → wait a random 1–3 s → re-read (`mcp__github__get_pull_request` / `get_issue`). Proceed ONLY if `assignees` is EXACTLY one entry and that entry is you. Empty, someone else, or MORE THAN ONE entry → another worker raced you → yield and restart selection. (`assignees` is a SET; "you are in the list" is NOT sufficient — require sole ownership.)
- **Claimable** = (a) no assignee, OR (b) a STALE claim, OR (c) already assigned to YOU by a crashed prior cycle (resume/redo it). A claim is STALE when the latest `🤖 [CLAIM]` comment is older than 30 min AND nothing happened since: for an issue — still no linked PR; for a PR under review — still no submitted review and no new push. Read claim age via `mcp__github__get_pull_request_comments` (PRs) or `gh api graphql` (`…comments(last:20){nodes{author{login} body createdAt}}`). Recover a stale claim by claiming it exactly as above. This reclaims work orphaned by a crashed cycle (assignment is a lease, not a permanent lock).

Priority — take the FIRST matching one, in this order (finishing beats starting):

1. CHANGES. A PR labeled `needs-work` where you are NOT the most recent pusher.
   - Find: `mcp__github__list_pull_requests` (state=open); for candidates read labels/commits via `mcp__github__get_pull_request`. Exclude any PR whose most-recent pusher is you.
   - If the round count (label `round-N`) is ≥ 3 → apply `needs-human`, remove `needs-work` (`mcp__github__update_issue` on the PR number with the new label set), skip this PR.
   - Otherwise → `[PICKED] type=address #<N>`; work type = address, PR = #N.

2. REVIEW. An open PR you are NOT the author of and NOT the most recent pusher, that is **claimable** (see above).
   - Find: `mcp__github__list_pull_requests` (state=open); details via `mcp__github__get_pull_request`.
   - Claim it **exclusively** (assign + `🤖 [CLAIM]` stamp + 1–3 s + re-read + sole-you check). Lost the race → yield, restart selection.
   - Otherwise → `[PICKED] type=review #<N>`; work type = review, PR = #N.

3. NEW ISSUE. An open issue labeled BOTH `task` AND `owner-agreed`, **claimable** (see above), whose blockers are ALL closed. An issue WITHOUT `owner-agreed` is ignored entirely — the owner has not signed off on it yet.
   - Find: `mcp__github__list_issues` (labels=["task","owner-agreed"], state=open).
   - Blockers: parse `Depends on #X` / `Blocked by #X` from the body. For each, `mcp__github__get_issue` → if any is open, skip this issue.
   - Claim it **exclusively** (assign + `🤖 [CLAIM]` stamp + 1–3 s + re-read + sole-you check). Lost the race → yield, restart selection. (Assigning yourself fires the `project-in-progress` automation that moves the card to In Progress — do NOT touch the board manually.)
   - Otherwise → `[PICKED] type=implement #<N>`; work type = implement, issue = #N.

DEADLOCK CIRCUIT BREAKER: if open `task`+`owner-agreed` issues exist but none qualify (all with open blockers / a suspected A↔B cycle), and there's no changes/review work → apply `needs-human` to the oldest blocked issue (`mcp__github__update_issue`), print `[SELECT] deadlock -> needs-human #<N>` then `WORK_QUEUE_EMPTY`, exit.

NO ELIGIBLE REVIEWER: if the only remaining work is REVIEW of PR(s) for which you are the author or the most-recent pusher (so you may not review them) and there is no other work → print `[SELECT] no-eligible-reviewer (need another account online)` then `WORK_QUEUE_EMPTY`, exit. Review needs all three machines/accounts running — an author cannot approve its own PR (see fleet-flow.md §Гейт).

If nothing was found → print `[SELECT] empty` then `WORK_QUEUE_EMPTY`, exit.
