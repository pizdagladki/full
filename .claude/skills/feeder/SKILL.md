---
name: feeder
description: Decompose the product spec under docs/ into many small, dependency-ordered GitHub issues for the work-cycle fleet. Human-gated via labels. Run explicitly; never auto-trigger.
disable-model-invocation: true
---
Turn the product spec into a dependency-ordered backlog of small GitHub issues for the `work-cycle` fleet. You do NOT implement anything and you do NOT run the fleet. Issues are created UNAPPROVED (`task` + `proposed`, never `owner-agreed`) — a human approves each one later by adding `owner-agreed`. This run is ONE-SHOT: plan, create, report, exit. Transport: GitHub issues via the **GitHub MCP** tools (`mcp__github__*`, owner `pizdagladki`, repo `full`); the Projects board via `gh project` (Bash).

`$ARGUMENTS` may contain a spec path (default `docs/`) and an optional filter `--area <name>` or `--flow <file>` to process a subset. Default = process the whole spec. An optional `--project <N>` names the Projects board to add issues to.

## Phase 0 — Read everything, build no side effects yet
1. Read the spec: the global `README`, the product overview, the user-flows doc, and EVERY individual flow doc (they live under `docs/specs/`). Read `docs/specs/tech-stack.md` (or `10-tech-stack.md`) and `docs/architecture.md` so areas map to real services.
2. Read `.github/ISSUE_TEMPLATE/task.md` — your issue bodies MUST match its sections exactly.
3. Read the skills `go-backend-conventions`, `new-service`, `new-resource` (backend) and `frontend-conventions` (frontend) so each task points at the right zone and implementation path. The stack is the canon there — `Echo` (`labstack/echo/v4`), `pgx`/PostgreSQL, `go-redis`, `minio-go`, `coder/websocket`, `golang-migrate`, `zap`, `validator/v10`, shared `internal/platform/{logger,postgres,redis,storage}`. Do NOT invent alternatives (no other web framework, no Mongo).
4. Inventory existing services: `ls services/*`. If the repo isn't built yet, derive the service set from the flows + tech stack (e.g. `auth`, `matchmaking`, `signaling`, `store`/payments, `profile`, `ratings`/match-history, `media`/WebM→MP4, `reports`). This service map is the source of zones (`Service / area`).
5. **Load the EXISTING backlog as FULL context — not just titles.** List ALL issues, open AND closed (`mcp__github__list_issues` / `mcp__github__search_issues`, repo `pizdagladki/full`), then read the COMPLETE body of EACH via `mcp__github__get_issue`: Goal, Acceptance criteria, Blocked by, Service/area, Out of scope, Context, and the hidden `<!-- fdr-* -->` fingerprint. This is the authoritative record of what already exists. Read full bodies (NOT titles) because tasks overlap — even more so here: you need each existing task's exact scope, acceptance criteria, zone and Out-of-scope to (a) never duplicate or re-cover already-issued work, (b) never contradict or collide with an existing task's zone/scope/boundaries, and (c) attach `Depends on #<real-number>` to the right existing issues. Carry this backlog into Phase 1 as the immovable baseline, and seed the Phase 2 `slug → issue#` map from these fingerprints.

## Phase 1 — Plan the task graph (in memory, no `gh` writes)
Decompose the spec into the SMALLEST implementable units. One issue = one PR-sized change = ONE zone (`services/<name>` or `frontend`). Typical unit kinds:
- **scaffold** — a new service skeleton (via `new-service`). One per service that doesn't exist yet.
- **migration** — a `golang-migrate` SQL change (paired `up`/`down`) for a feature's tables.
- **slice** — one resource as a vertical slice (domain→repository→service→delivery→app→routes, via `new-resource`).
- **endpoint / worker / integration** — a single WS handler, a background worker (matchmaking loop, WebM→MP4 via ffmpeg), or wiring an external provider behind its interface (Stripe `PaymentProvider`, Google OAuth, MinIO storage).
- **frontend** — one React area/component or isolated module, via the `frontend-conventions` skill. The fleet now builds & fixes frontend, so frontend units MAY be emitted; their Service/area zone is `frontend` (a valid zone alongside `services/<name>`).

For each unit produce: title, Goal, **verifiable** Acceptance criteria, Service/area (a concrete zone like `services/auth`), Out of scope, Context (source doc + which skill to use). Acceptance criteria must be testable AS WRITTEN — e.g. "`POST /v1/auth/google` with a valid code → 200 + session in Redis; invalid code → 401", NOT "auth works". `work-cycle`'s `implement.md` step 1 bounces unverifiable criteria to `needs-human`, so vague criteria = a wasted round.

Dependencies (build a DAG with internal slug IDs; reference by `Depends on #<slug>`):
- scaffold-before-any-slice-in-that-service; migration-before-the-feature-using-it; users/auth + DB schema before anything user-scoped; backend endpoint before the frontend that consumes it.
- Detect cycles; if two units are mutually dependent, merge them or split differently. No cycles.

MANUAL prerequisites are NOT fleet issues. Things like creating the Google OAuth client, the Stripe account/keys, provisioning Postgres/Redis/MinIO, DNS/TURN — do NOT create issues for these. Instead, in the Context of any task that needs them, add a line `Manual prerequisite (human): <what>` so the human knows to satisfy it before approving that task.

**Build on the existing backlog (from Phase 0 step 5) — do not restart it.** Treat every already-existing issue as a fixed node in the graph. Plan ONLY genuinely new units beyond it: if a unit you would emit is already covered by an existing issue — judged from its FULL body (matching fingerprint, OR overlapping scope / acceptance criteria / zone), NOT its title alone — drop it and reuse that issue's number. New units must respect existing tasks' zones and Out-of-scope so they don't overlap. Wire `Depends on` from new units onto the real numbers of the existing issues they build on, exactly as for new-to-new dependencies. When a run is capped to N issues, N counts NEW issues created — skipped/existing ones do not count.

Topologically sort the units (dependencies first).

## Phase 2 — Emit issues, idempotently, in topological order
Keep a map `slug → issue#`, pre-seeded from the existing issues loaded in Phase 0 step 5 (their `fdr-*` fingerprints → their real numbers) so `Depends on` onto existing work resolves and nothing already-issued is recreated. For each unit in order:
1. Compute a stable fingerprint `fdr-<area>-<short-slug>` (kebab-case, unique).
2. Check existence ACROSS ALL ISSUES regardless of state or label (an approved issue has lost `proposed`; a rejected one is closed — do NOT re-create either):
   `mcp__github__search_issues` with query `repo:pizdagladki/full type:issue fdr-<area>-<short-slug> in:body` (no state filter → searches open AND closed).
   - If found (by fingerprint) OR already covered by an existing issue from the Phase 0 backlog (by scope/meaning, from its full body) → record its number in the map, SKIP creation.
   - If not found → create it.
3. Create with the template body, resolving every `Depends on #<slug>` to the real `#number` from the map (topological order guarantees deps already exist in the map). Always include the hidden fingerprint as the LAST body line: `<!-- fdr-<area>-<short-slug> -->`. Labels: `task,proposed`. Capture the new number into the map.
   `mcp__github__create_issue` (owner `pizdagladki`, repo `full`, title, body, labels=["task","proposed"]).
4. If a Projects board number is configured (`--project <N>` or your Context), add the issue to it so `implement.md` can move its card: `gh project item-add <N> --owner pizdagladki --url <issue-url>`. If no board is configured, skip.

Issue body (match `.github/ISSUE_TEMPLATE/task.md` section-for-section):
```
## Goal
<1–2 sentences>

## Acceptance criteria
- [ ] <verifiable> …

## Blocked by
- Depends on #<n>        (if no blockers, leave this section empty — no Depends line)

## Service / area
services/<name>          (the single zone this task may touch)

## Out of scope
- <what NOT to do> …

## Context
Source: docs/specs/<flow>.md
Implement with: new-resource | new-service skill; follow go-backend-conventions.
Manual prerequisite (human): <only if any>

<!-- fdr-<area>-<short-slug> -->
```

## Phase 3 — Report and exit
Print to stdout: counts (created vs skipped-existing), the dependency graph (slug → #number → its deps), and a list of every `Manual prerequisite (human)` you emitted, grouped, so the human knows what to set up before approving the affected issues. Remind that nothing is in the fleet's queue until a human adds `owner-agreed`, and that dependencies must be approved in order (a task whose blocker is unapproved will wait; `work-cycle`'s deadlock breaker eventually flags it `needs-human`).

Then EXIT. Do not loop. Do not implement. Do not add `owner-agreed`. Do not run `work-cycle`.
