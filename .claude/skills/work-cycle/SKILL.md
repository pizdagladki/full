---
name: work-cycle
description: One cycle of an autonomous worker — pick work from the GitHub queue, do it, exit.
disable-model-invocation: true
---
Run EXACTLY ONE cycle and exit. Hold no state in memory — everything lives in GitHub. Print `[CYCLE] start` first.

ALWAYS `git fetch origin` and work from the latest remote state before touching code — never a stale local branch. Each step below re-syncs the exact branch it needs (implement → fresh `origin/main`; address → the PR branch's latest remote head; review → the PR's current head).

## Transport — which tool for what
- **GitHub issues & PRs** (get/create/update issue, comment, set labels/assignee, get PR, PR files/diff, PR status, create PR, create PR review) → the **GitHub MCP** server tools (`mcp__github__*`). Owner `pizdagladki`, repo `full`. Confirm exact tool names against your available tools; do NOT invent.
- **Projects board** → automated by GitHub workflows; the skills do NOT move cards (see below).
- **Enable auto-merge** → `gh pr merge <N> --auto --squash` (Bash). NEVER use the MCP merge tool anywhere — it merges immediately and bypasses the wait-for-green-CI model.
- **Local code** (git worktree/rebase/push, `make` test/lint, gofmt) → Bash.

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
   - review a PR → `steps/review.md`
   - new issue → `steps/implement.md`
3. Before exiting print `[CYCLE] done type=<address|review|implement>`, then a final recap `[CYCLE] end #<N>`. After finishing the unit — exit the process. Do NOT take a second unit in the same invocation (the next invocation starts with a clean context).
