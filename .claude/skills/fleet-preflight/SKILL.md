---
name: fleet-preflight
description: One-shot per-machine startup check for the work-cycle fleet — verifies this machine's GitHub identity (gh/git == MCP, and distinct across the 3 machines), MCP reachability, and the merge-gate prerequisites. Run BEFORE starting the work-cycle loop; never auto-trigger.
disable-model-invocation: true
---
Verify this machine is safe to join the fleet, then EXIT. Do NOT pick work, implement, or run `work-cycle`. Print `[PREFLIGHT] start <FLEET_AGENT>`. Owner `pizdagladki`, repo `full`.

The fleet's "no self-merge" guarantee rests on THREE genuinely distinct GitHub identities — one per machine — used consistently for both git/gh pushes and the GitHub MCP. Nothing else enforces this at runtime; this skill does. Any `PREFLIGHT_FAIL …` means do NOT start the loop on this machine until it is fixed.

## 1. Identity — gh/git side
- `gh api graphql -f query='{viewer{login}}'` (Bash) → the account your **git push / gh** operations act as. Call it `GH_LOGIN`. If it errors → `PREFLIGHT_FAIL gh-auth` and exit.

## 2. Identity — MCP side, and gh == MCP on this machine
- Find/create the shared marker issue (fingerprint `<!-- fleet-preflight-marker -->`): `mcp__github__search_issues` query `repo:pizdagladki/full in:body fleet-preflight-marker`. If absent → `mcp__github__create_issue` title `fleet-preflight`, body containing that fingerprint and NO `task`/`owner-agreed` labels (it must stay invisible to select.md).
- Post `mcp__github__add_issue_comment` on the marker: `🤖 [PREFLIGHT] <FLEET_AGENT> <UTC-ISO8601>`. The author of this comment is your **MCP identity** → `MCP_LOGIN` (read it back via `mcp__github__get_issue` / `gh api graphql` `…comments(last:30){nodes{author{login} body createdAt}}`).
- GATE: if `GH_LOGIN != MCP_LOGIN` → your gh/git account and your MCP token are DIFFERENT accounts; `select.md`'s "most-recent-pusher is you" and the merge-gate author check will misfire. Print `PREFLIGHT_FAIL identity-mismatch gh=<GH_LOGIN> mcp=<MCP_LOGIN>` and exit.

## 3. Distinct across machines
- From the marker's recent (last 24 h) `[PREFLIGHT]` comments, collect the set of distinct author logins. Expect one per machine.
- If two different `FLEET_AGENT`s report the SAME login → `PREFLIGHT_FAIL shared-identity <login>` and exit (the "approval from a different account" gate would collapse — a single account could author, approve and merge).
- If fewer machines have checked in than you run → `PREFLIGHT_WARN identities=<n> logins=<…>` (review work cannot progress until ≥2 other accounts are online — see select.md NO ELIGIBLE REVIEWER).

## 4. MCP reachability + merge-gate prerequisites
- MCP reachability is already exercised above (search/create/comment). If any of those errored → `PREFLIGHT_FAIL mcp-unreachable`.
- Copilot bot login: from the most recent open PR with review threads (`gh api graphql` reviewThreads), print the bot author login so the operator can confirm it matches what `review.md` matches on. None yet → `PREFLIGHT_WARN copilot-login-unconfirmed`.
- Required checks (GitHub-side, cannot be fully read here): the fleet must NOT run until the branch-protection ruleset requires EVERY CI job (`lint typecheck test build frontend spell spell-diff`). Print `[PREFLIGHT] confirm required checks in ruleset` as an operator reminder. (review.md also verifies per-job `success` at runtime as a backstop.)
- `typos --version` (Bash): on failure → `PREFLIGHT_WARN typos-missing` (the local spell hook becomes a silent no-op; CI still gates).

## 5. Verdict
- No FAIL → print `[PREFLIGHT] ok <FLEET_AGENT> login=<GH_LOGIN>` and EXIT. Otherwise leave the `PREFLIGHT_FAIL …` line as the last output and EXIT without starting the loop.
