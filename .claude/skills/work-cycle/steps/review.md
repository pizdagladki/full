Review PR #N (you claimed it by assigning yourself; you are NOT the author and NOT the most recent pusher). Print `[REVIEW] #<N> start`.

1. **Sync first:** `git fetch origin`, then confirm you are reviewing the **latest pushed head** — `mcp__github__get_pull_request` (#N), compare `head.sha`; if it changed since you claimed, re-read. Fetch the diff via `mcp__github__get_pull_request_files` (#N). Check CI status via `mcp__github__get_pull_request_status` (#N) (fallback: `gh pr checks <N>` on Bash). Read the linked issue's acceptance criteria via `mcp__github__get_issue`.
2. **Copilot comments (advisory — must be adjudicated AND resolved):** fetch existing review comments via `mcp__github__get_pull_request_comments` (#N), AND list the resolvable threads (you need their IDs) via GraphQL:
   `gh api graphql -f query='query($o:String!,$r:String!,$n:Int!){repository(owner:$o,name:$r){pullRequest(number:$n){reviewThreads(first:100){nodes{id isResolved comments(first:1){nodes{author{login} body path line}}}}}}}' -F o=pizdagladki -F r=full -F n=<N>`
   SEPARATE OUT the threads authored by the **Copilot reviewer bot** — match on bot author (type `Bot`, login like `copilot-pull-request-reviewer`; confirm the exact login on a real PR). Collect each Copilot thread's `id` + body + file/line. If Copilot's review isn't present yet → print `[COPILOT] none yet` and proceed — NEVER block waiting for Copilot.
3. **Delegate** the diff + PR metadata + the issue's acceptance criteria + the list of Copilot comments to BOTH the `code-reviewer` AND the `security-reviewer` subagents (fresh context; they review what you pass in — they do NOT fetch anything). Instruct the `code-reviewer` to classify EACH Copilot comment as `apply` (a real correctness/criteria/security defect) or `dismiss` (style-only / false positive / out of scope) WITH a one-line reason. Copilot is advisory and treated with caution: a Copilot comment alone is NOT enough to fail the PR unless it pinpoints a genuine correctness/criteria/security problem; never apply it blindly. Each reviewer returns its own verdict PLUS — from the code-reviewer — the per-Copilot-comment adjudication list.
4. **Resolve EVERY Copilot thread** (resolution ≠ application — done in BOTH the GOOD and BAD paths, so require-conversation-resolution can't silently block auto-merge). For each Copilot thread:
   - `apply` → reply briefly that it's folded into the requested changes (`mcp__github__add_issue_comment` #N), then resolve.
   - `dismiss` → post the one-line reason (`mcp__github__add_issue_comment` #N), then resolve.
   - Resolve (no MCP/plain-gh equivalent): `gh api graphql -f query='mutation($t:ID!){resolveReviewThread(input:{threadId:$t}){thread{isResolved}}}' -F t=<threadId>`.
   Print `[COPILOT] adjudicated apply=<a> dismiss=<d> resolved=<r>`.
5. Form the verdict from both reviews. Red CI = an automatic BAD regardless of the code.
   - **GOOD** = your reviewers find no blocking issue AND there are NO `apply` Copilot findings AND CI is green/going green. Print `[REVIEW] #<N> verdict=GOOD`.
     - Approve via `mcp__github__create_pull_request_review` (#N, event=APPROVE, body=summary).
     - Enable auto-merge: `gh pr merge <N> --auto --squash` (Bash). NEVER use the MCP merge tool.
     - Board stays `In Review` (the built-in `PR linked → In Review` Projects workflow handles it) — do NOT set Done. Print `[REVIEW] #<N> approved automerge`.
   - **BAD** = your reviewers find a blocking issue OR there is ≥ 1 `apply` Copilot finding. Print `[REVIEW] #<N> verdict=BAD round=<n>`.
     - Post specific, self-contained line comments via `mcp__github__create_pull_request_review` (#N, event=REQUEST_CHANGES, with the `comments` line refs) or `mcp__github__add_issue_comment` (#N) — **including the applied Copilot findings rephrased as your OWN actionable comments** (a fresh agent resolves them without your context).
     - Round counter: read the PR's labels (`mcp__github__get_pull_request`); if no `round-*` label → add `round-1`; otherwise replace `round-N` with `round-(N+1)` — `mcp__github__update_issue` on #N.
     - Apply `needs-work` (`mcp__github__update_issue`). At round ≥ 3 → apply `needs-human` instead of `needs-work` and print `[REVIEW] #<N> escalated needs-human`.
     - RELEASE your reviewer claim: remove yourself from assignees (`mcp__github__update_issue` on #N, assignees without you) so the PR returns to the unclaimed "awaiting review" pool (a fresh agent re-reviews after the changes).
     - Requesting changes triggers the built-in `code changes requested → In Progress` Projects workflow — no manual board move. Print `[REVIEW] #<N> needs-work round=<n>`.
6. Exit.
