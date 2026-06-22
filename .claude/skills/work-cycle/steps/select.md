Pick EXACTLY ONE unit of work. Print `[SELECT] scanning`. GitHub reads/writes use the **GitHub MCP** tools (`mcp__github__*`, owner `pizdagladki`, repo `full`).

0. KILL-SWITCH: `mcp__github__list_issues` (labels=["fleet-stop"], state=open). If any open issue is labeled `fleet-stop` → print `[SELECT] fleet-stop` then `WORK_QUEUE_EMPTY` and exit. The fleet has been stopped by a human.

CLAIMING IS A LEASE, NOT A LOCK — read before the cascade:
- **Exclusive claim.** To claim a PR/issue: `mcp__github__issue_write` (method=update) assignees=[you] AND post a stamp `mcp__github__add_issue_comment` (#N) `🤖 [CLAIM] <FLEET_AGENT> <UTC-ISO8601>` → wait a random 1–3 s → re-read (`mcp__github__pull_request_read` / `mcp__github__issue_read`, method=get). Proceed ONLY if `assignees` is EXACTLY one entry and that entry is you. Empty, someone else, or MORE THAN ONE entry → another worker raced you → yield and restart selection. (`assignees` is a SET; "you are in the list" is NOT sufficient — require sole ownership.)
- **Claimable** = (a) no assignee, OR (b) a STALE claim, OR (c) already assigned to YOU by a crashed prior cycle (resume/redo it). A claim is STALE when the latest `🤖 [CLAIM]` comment is older than 30 min AND nothing has happened SINCE that claim comment: no new commit/push to the branch, no review submitted after it, and no label change after it (to check label changes, query the PR/issue timeline events via `gh api graphql` `timelineItems` including `LABELED_EVENT`/`UNLABELED_EVENT` and compare `createdAt` to the claim timestamp; if you can’t retrieve timeline events, ignore label changes for staleness). Judge progress RELATIVE TO the claim's timestamp, NOT the mere existence of an older review — an approval from a previous round must never keep a fresh claim alive forever. Read claim age via `mcp__github__pull_request_read` (method=get_comments, PRs) or `gh api graphql` (`…comments(last:20){nodes{author{login} body createdAt}}`). Recover a stale claim by claiming it exactly as above. This reclaims work orphaned by a crashed cycle (assignment is a lease, not a permanent lock).

Priority — take the FIRST matching one, in this order (finishing beats starting):

1. CHANGES. A PR labeled `needs-work` where you are NOT the most recent pusher.
   - Find: `mcp__github__list_pull_requests` (state=open); for candidates read labels/commits via `mcp__github__pull_request_read` (method=get). Exclude any PR whose most-recent pusher is you.
   - If the round count (label `round-N`) is ≥ 3 → apply `needs-human`, remove `needs-work` (`mcp__github__issue_write` method=update on the PR number with the new label set), skip this PR.
   - Otherwise → `[PICKED] type=address #<N>`; work type = address, PR = #N.

2. UNBLOCK. An open PR that GitHub will never merge on its own because its branch is no longer mergeable — even though the fleet may have already reviewed it. This un-sticks two cases once `main` moves under a branch: (a) an approved + auto-merge-armed PR that would otherwise hang forever behind `main` after the first PR of a batch merges; and (b) a PR that fell behind `main` while still awaiting (re-)review — a reviewer can neither approve nor make a review stick on a `BEHIND` branch, so without a mechanical re-sync here it ping-pongs forever between REVIEW (which defers it as good-but-unmergeable) and a UNBLOCK that used to ignore it (see fleet-flow.md §Гейт). Consider only PRs NOT labeled `needs-work` (a `needs-work` PR belongs to CHANGES → address.md, which already merges `main` and resolves conflicts itself). For each remaining open PR read `gh pr view <N> --json mergeStateStatus,mergeable,reviewDecision,autoMergeRequest,headRefName,isDraft` (Bash). It matches when ANY of:
   - `mergeable == CONFLICTING` (state `DIRTY`) — a real conflict with `main`, at ANY review state; OR
   - `mergeStateStatus == BEHIND` (and `mergeable != CONFLICTING`) — behind `main` at ANY review state (approved, `REVIEW_REQUIRED`, or `CHANGES_REQUESTED`): the branch needs `main` merged in before any review can stick or auto-merge can fire. Mechanical re-sync, NOT a review — no approval happens; OR
   - `reviewDecision == APPROVED` AND `mergeStateStatus` is `CLEAN`/`UNSTABLE`/`BLOCKED` AND `autoMergeRequest == null` — approved but auto-merge was never actually armed (it just needs re-arming; harmless to arm while `BLOCKED` — it fires once the block clears).
   Skip drafts; treat `UNKNOWN` as "not yet — let GitHub finish computing" (re-read once, else skip). You MAY pick your OWN PR here — this is mechanical re-sync, not review, so no self-approval happens. (A PR that is currently claimed by an active reviewer is not claimable → you will yield, so this never collides with an in-flight review.)
   - Claim it **exclusively** (assign + `🤖 [CLAIM]` stamp + 1–3 s + re-read + sole-you check). Lost the race → yield, restart selection.
   - Otherwise → `[PICKED] type=unblock #<N>`; work type = unblock, PR = #N.

3. REVIEW. An open PR you are NOT the author of and NOT the most recent pusher, NOT labeled `needs-work` (those go to CHANGES above), that still NEEDS a (re-)review decision — `reviewDecision` is anything OTHER than `APPROVED` (i.e. `REVIEW_REQUIRED`, `CHANGES_REQUESTED`, or unset). Crucially `CHANGES_REQUESTED` is included: after `address.md` fixes a PR and removes `needs-work`, the prior `REQUEST_CHANGES` review still stands (unless the repo auto-dismisses stale reviews), so the PR sits at `CHANGES_REQUESTED` with no `needs-work` — it MUST be re-reviewable here or it would be orphaned. An `APPROVED` PR is excluded (it is either on its way to merge or handled by UNBLOCK above). Must be **claimable** (see above).
   - Find: `mcp__github__list_pull_requests` (state=open); details via `mcp__github__pull_request_read` (method=get) + `gh pr view <N> --json reviewDecision` for the decision.
   - Claim it **exclusively** (assign + `🤖 [CLAIM]` stamp + 1–3 s + re-read + sole-you check). Lost the race → yield, restart selection.
   - Otherwise → `[PICKED] type=review #<N>`; work type = review, PR = #N.

4. NEW ISSUE. An open issue labeled BOTH `task` AND `owner-agreed`, **claimable** (see above), whose blockers are ALL closed. An issue WITHOUT `owner-agreed` is ignored entirely — the owner has not signed off on it yet.
   - Find: `mcp__github__list_issues` (labels=["task","owner-agreed"], state=open).
   - Blockers: parse `Depends on #X` / `Blocked by #X` from the body. For each, `mcp__github__issue_read` (method=get) → if any is open, skip this issue.
   - Claim it **exclusively** (assign + `🤖 [CLAIM]` stamp + 1–3 s + re-read + sole-you check). Lost the race → yield, restart selection. (Assigning yourself fires the `project-in-progress` automation that moves the card to In Progress — do NOT touch the board manually.)
   - Otherwise → `[PICKED] type=implement #<N>`; work type = implement, issue = #N.

DEADLOCK CIRCUIT BREAKER: if open `task`+`owner-agreed` issues exist but none qualify (all with open blockers / a suspected A↔B cycle), and there's no changes/unblock/review work → apply `needs-human` to the oldest blocked issue (`mcp__github__issue_write` method=update), print `[SELECT] deadlock -> needs-human #<N>` then `WORK_QUEUE_EMPTY`, exit.

NO ELIGIBLE REVIEWER: if the only remaining work is REVIEW of PR(s) for which you are the author or the most-recent pusher (so you may not review them) and there is no other work → print `[SELECT] no-eligible-reviewer (need another account online)` then `WORK_QUEUE_EMPTY`, exit. Review needs all three machines/accounts running — an author cannot approve its own PR (see fleet-flow.md §Гейт).

If nothing was found → print `[SELECT] empty` then `WORK_QUEUE_EMPTY`, exit.
