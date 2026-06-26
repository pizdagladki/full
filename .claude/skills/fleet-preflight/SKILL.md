---
name: fleet-preflight
description: One-shot startup check for the single-agent work-cycle — verifies the GitHub identity (gh/git == MCP), MCP reachability, and the single-agent merge-gate prerequisites (ruleset must require CI but NOT approvals). Run BEFORE starting the work-cycle loop; never auto-trigger.
disable-model-invocation: true
---
Verify this machine is safe to run the work-cycle, then EXIT. Do NOT pick work, implement, or run `work-cycle`. Print `[PREFLIGHT] start <FLEET_AGENT>`. Owner `pizdagladki`, repo `full`.

SINGLE-AGENT MODE: the fleet now runs as ONE agent on ONE account. It reviews and merges its OWN PRs — there is no cross-account "no self-merge" guarantee anymore; objectivity rests on CI + the criterion→test gate + the fresh-process self-review (see fleet-flow.md §Гейт), NOT on a second identity. What this skill still must confirm: one consistent identity for gh/git AND MCP, MCP reachability, and that the branch-protection ruleset is configured for single-account merge (requires CI, does NOT require approvals). Any `PREFLIGHT_FAIL …` means do NOT start the loop until it is fixed.

## 1. Identity — gh/git side
- `gh api graphql -f query='{viewer{login}}'` (Bash) → the account your **git push / gh** operations act as. Call it `GH_LOGIN`. If it errors → `PREFLIGHT_FAIL gh-auth` and exit.

## 2. Identity — MCP side, and gh == MCP on this machine
- Find/create the shared marker issue (fingerprint `<!-- fleet-preflight-marker -->`): `mcp__github__search_issues` query `repo:pizdagladki/full in:body fleet-preflight-marker`. If absent → `mcp__github__create_issue` title `fleet-preflight`, body containing that fingerprint and NO `task`/`owner-agreed` labels (it must stay invisible to select.md).
- Post `mcp__github__add_issue_comment` on the marker: `🤖 [PREFLIGHT] <FLEET_AGENT> <UTC-ISO8601>`. The author of this comment is your **MCP identity** → `MCP_LOGIN` (read it back via `mcp__github__get_issue` / `gh api graphql` `…comments(last:30){nodes{author{login} body createdAt}}`).
- GATE: if `GH_LOGIN != MCP_LOGIN` → your gh/git account and your MCP token are DIFFERENT accounts; `select.md`'s "most-recent-pusher is you" and the merge-gate author check will misfire. Print `PREFLIGHT_FAIL identity-mismatch gh=<GH_LOGIN> mcp=<MCP_LOGIN>` and exit.

## 3. Single-account merge gate (was: distinct across machines)
- Single-agent has no second identity to check against, so there is no `shared-identity` / `no-eligible-reviewer` gate anymore (select.md no longer has that exit). Instead, confirm the OPPOSITE prerequisite: the ruleset must let one account merge its own PR.
- The ruleset MUST NOT require pull-request approvals (and MUST NOT require "approval of most recent push"). GitHub forbids approving your own PR, so if approvals are required, EVERY PR hangs forever. This cannot be fully read from here — print `[PREFLIGHT] confirm ruleset requires NO approvals (single-account self-merge)` as an operator reminder. If you CAN read it (`gh api repos/pizdagladki/full/rulesets` or branch protection) and an approval count ≥ 1 is required → `PREFLIGHT_FAIL ruleset-requires-approval` and exit.

## 4. MCP reachability + merge-gate prerequisites
- MCP reachability is already exercised above (search/create/comment). If any of those errored → `PREFLIGHT_FAIL mcp-unreachable`.
- Copilot bot login: from the most recent open PR with review threads (`gh api graphql` reviewThreads), print the bot author login so the operator can confirm it matches what `review.md` matches on. None yet → `PREFLIGHT_WARN copilot-login-unconfirmed`.
- Required checks (GitHub-side, cannot be fully read here): the loop must NOT run until the branch-protection ruleset requires EVERY CI job (`lint typecheck test build frontend`) AND require-conversation-resolution is on (these are the binding merge gate now). Print `[PREFLIGHT] confirm required checks + conversation-resolution in ruleset` as an operator reminder. (review.md also verifies per-job `success` at runtime as a backstop.)

## 5. Verdict
- No FAIL → print `[PREFLIGHT] ok <FLEET_AGENT> login=<GH_LOGIN>` and EXIT. Otherwise leave the `PREFLIGHT_FAIL …` line as the last output and EXIT without starting the loop.
