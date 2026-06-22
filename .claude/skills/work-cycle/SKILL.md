---
name: work-cycle
description: One cycle of an autonomous worker — pick work from the GitHub queue, do it, exit.
disable-model-invocation: true
---
Run EXACTLY ONE cycle and exit. Hold no state in memory — everything lives in GitHub. Print `[CYCLE] start` first.

ALWAYS `git fetch origin` and work from the latest remote state before touching code — never a stale local branch. Each step below re-syncs the exact branch it needs (implement → fresh `origin/main`; address → the PR branch's latest remote head; unblock → merges `main` into a behind/conflicting PR branch; review → the PR's current head).

## Transport — which tool for what
- **GitHub issues & PRs** → the **GitHub MCP** server tools (`mcp__github__*`), owner `pizdagladki`, repo `full`. Current tool names in this build (confirm against your available tools; do NOT invent):
  - `issue_read` (`method=get`/`get_comments`/`get_labels`/`get_sub_issues`), `issue_write` (`method=create`/`update` — sets `labels`/`assignees`/`state`/`body`), `add_issue_comment`, `list_issues`, `search_issues`.
  - `pull_request_read` (`method=get`/`get_diff`/`get_files`/`get_status`/`get_check_runs`/`get_comments`/`get_review_comments`/`get_reviews`), `list_pull_requests`, `create_pull_request`.
  - `pull_request_review_write` (`method=create` with `event=APPROVE`/`REQUEST_CHANGES`/`COMMENT`; or `method=resolve_thread`/`unresolve_thread` with `threadId`).
- **Projects board** → automated by GitHub workflows; the skills do NOT move cards (see below).
- **Enable auto-merge** → `gh pr merge <N> --auto --squash` (Bash). NEVER use the MCP merge tool anywhere — it merges immediately and bypasses the wait-for-green-CI model.
- **Bring a behind branch up to date** → `gh pr update-branch <N>` (Bash; merges `main` into the PR branch — no force-push) or `git merge origin/main` in a worktree. NEVER rebase-and-force-push a pushed PR branch: force-push is denied (`settings.json`) and blocked by branch protection, so rebasing it is a dead end.
- **Local code** (git worktree/merge/push/reset, `make` test/lint, gofmt) → Bash. (Sync is always a merge — never rebase-and-force-push a pushed branch; force-push is denied.)

## Projects board (automated — do not touch from the skills)
Board status is driven by automation, not by the work-cycle steps:
- **issue assigned → In Progress** — the `project-in-progress.yml` repo workflow (there is no built-in for "assigned"), fired when a worker claims an `owner-agreed` task (the assignment in `select.md`).
- **Todo / In Review / Done** — the Projects board's **built-in** workflows (Auto-add → Todo, PR linked → In Review, merged/closed → Done, reopened → Todo), configured by the human in the Project's Workflows UI — NOT in `.github/`.
So the assignment + the PR/issue lifecycle move the card; the steps issue NO `gh project` status edits. (New issues are placed on the board by the `feeder` skill via `gh project item-add`.)

## Cycle
1. Read `steps/select.md` and pick ONE unit of work.
   - If there's no work → print exactly `WORK_QUEUE_EMPTY` and exit. (The outer wrapper stops the loop on this word.)
2. Depending on the type of the chosen work, run EXACTLY ONE step:
   - changes requested (needs-work) → `steps/address.md`
   - unblock (a PR GitHub won't merge on its own — behind `main` at any review state, conflicting with `main`, or approved-but-auto-merge-not-armed) → `steps/unblock.md`
   - review a PR → `steps/review.md`
   - new issue → `steps/implement.md`
3. Before exiting print `[CYCLE] done type=<address|unblock|review|implement>`, then a final recap `[CYCLE] end #<N>`. After finishing the unit — exit the process. Do NOT take a second unit in the same invocation (the next invocation starts with a clean context).

## Failure vs empty — distinct exits (so the wrapper can tell a drained queue from a crash)
- `WORK_QUEUE_EMPTY` means ONLY "nothing to do" (or the `fleet-stop` kill-switch). NEVER print it on an error.
- If any required tool/MCP call fails unrecoverably (tool not found, GitHub API error after one retry, `git push` / PR-create failure), print a single line `CYCLE_ERROR <step> <short-reason>` and exit. NEVER swallow a failure and fall through to `WORK_QUEUE_EMPTY`. State lives in GitHub, so the next cycle re-reads and recovers; the marker just lets the operator see this cycle aborted (the wrapper stops loudly on it).

## Run once per machine BEFORE the loop
Run the `fleet-preflight` skill once per machine before starting the `/work-cycle` loop — it verifies this machine's GitHub identity (gh/git == MCP, and distinct from the other two machines), MCP reachability, and the merge-gate prerequisites. A `PREFLIGHT_FAIL …` means do NOT start the loop here until fixed.
