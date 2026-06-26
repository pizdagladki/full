# feeder

The missing **front half** of the autonomous pipeline. `feeder` turns the product spec into a
dependency-ordered backlog of small GitHub issues; the `work-cycle` fleet drains that backlog into PRs.

```
spec (docs/specs/) → /feeder → issues (task + proposed)
   → human adds `owner-agreed` (triage gate)
   → fleet (/work-cycle): select → implement → review → merge
   → merge closes the issue → unblocks its dependents
```

`feeder` is the planner; `work-cycle` is the worker. The skill itself is `SKILL.md`; this file is the
operator's guide.

## What it does

One run = the four phases in `SKILL.md`:

- **Phase 0 — read.** README + overview + user-flows + every flow doc under `docs/specs/`, the tech-stack
  doc, `docs/architecture.md`, the issue template, and the `go-backend-conventions` / `new-service` /
  `new-resource` skills. Builds the service map (from `services/*`, or derived from the flows when the repo
  is still empty).
- **Phase 1 — plan.** Cuts each business flow into the smallest implementable units (scaffold, migration,
  vertical slice, endpoint/worker/integration, frontend) and builds a dependency DAG (`Depends on #N`):
  scaffold before slices, migration before the feature, auth + schema before anything user-scoped, backend
  before the frontend that consumes it. No cycles. Manual ops (OAuth client, Stripe keys, infra) are NOT
  issues — they're noted as `Manual prerequisite (human): …` in the relevant task's Context.
- **Phase 2 — emit.** In topological order, creates issues with `task,proposed` and the real resolved
  `Depends on #N`, hiding a stable fingerprint `<!-- fdr-<area>-<slug> -->` in each body. Before creating it
  searches **all** issues for that fingerprint. If a unit overlaps an existing issue it does NOT blindly
  skip — it diffs the updated-spec criteria against that issue's: unchanged → skip; **diverged + the issue
  is closed/merged → emit a `…-reconcile` task** (a forward modification on top of the shipped code);
  diverged + the issue is still open → flag it for human re-triage.
- **Phase 3 — report.** Created (new vs reconcile) vs skipped counts, the dependency graph, the
  `[RE-TRIAGE]` list of open issues whose criteria drifted, and the grouped manual prerequisites. Then exits.

## Why it's idempotent (survives crashes, re-run on every spec change)

State lives in **GitHub, not the agent's memory**. The fingerprint in each body is a stable anchor, and the
existence check scans issues of **any** state/label on purpose — so a re-run recreates neither an approved
issue (it has lost `proposed`) nor a rejected one (it's `closed`). Run it again whenever the spec changes:
for units it already issued it tops up only what's NEW, and where the spec has MOVED under already-shipped
code it emits a `reconcile` task (or flags an open task for re-triage) instead of silently missing the change.
It does NOT diff the spec against the CODE — only against existing issues' criteria — so it catches spec
drift on things it has an issue for, not undocumented drift.

## Run it (one-shot, never a loop)

Unlike `work-cycle`, `feeder` is a single pass — no `while`:

```bash
# Whole spec. Idempotent — safe to repeat after a spec change.
claude -p "/feeder docs/" \
    --permission-mode <you-choose> \
    --model opus \
    --effort xhigh

# A slice of the spec, so triage stays human-sized:
claude -p "/feeder docs/ --area auth"
```

- **High effort on purpose.** Decomposition + dependency-graph reasoning is reasoning-heavy, and the run is
  one-shot — `xhigh` (or ultracode) is justified here, unlike the fleet's endless loop.
- **Pipeline order:** feeder → (human triage: `owner-agreed`) → fleet. Don't point `/work-cycle` at fresh
  `proposed` issues — it ignores anything without `owner-agreed` anyway; the point is the human checks first.
- `feeder` only needs `gh` with issue/project rights — it reviews nothing and runs no fleet.

## How to get the spec into `docs/`

The flow docs live **in the repo** at `docs/specs/` (the issue template's Context points there, and the
fleet reads them — it cannot see anything outside the repo). Put them there on the `feat/add-specs` branch:

```bash
# from the repo root, copy the PM spec into docs/specs/ and commit on feat/add-specs
cp /path/to/project-overview/*.md docs/specs/
git add docs/specs && git commit -m "docs: add product spec under docs/specs"
```

Recommended `docs/specs/` layout (keep the numbering — it gives feeder a reading order):

```
docs/specs/
├── README.md            # index + responsibility-zone rules
├── 00-overview.md
├── 01-user-flow.md
├── 02-…-09-…md           # one file per flow
├── 10-tech-stack.md
└── tech-stack.md        # distilled canonical stack (matches CLAUDE.md + go-backend-conventions)
```

Keep flow docs concrete: **backlog quality = spec quality.** Vague flows yield vague acceptance criteria,
which the human bounces at triage or `implement.md` bounces to `needs-human`. The more concrete the
acceptance criteria already are in the spec, the less manual rejection downstream.

## One-time human setup

1. **Create the `proposed` label** (the others — `task`, `owner-agreed` — already exist):
   ```bash
   gh label create proposed -d "feeder-created, awaiting owner triage"
   ```
2. **Spec in `docs/specs/`** — readable in the repo (see above).
3. **Triage is your gate.** After a run: `gh issue list --label proposed`, add `owner-agreed` to the good
   ones **in dependency order** (blockers first — scaffolds, migrations, auth — then dependents); close the
   bad ones (feeder won't recreate them). Approving a dependent before its blocker just makes the fleet wait.
4. **(Optional) a Projects board** — pass `--project <N>` so feeder places issues on the board; the
   `project-in-progress` GitHub workflow then moves a card to In Progress when a worker claims (assigns) the
   issue. No board → issues just aren't tracked there.
5. **`gh auth`** done; the **GitHub MCP** server connected (issues/PRs) and `gh project:*` allowlisted for the board (already in `.claude/settings.json`).
6. **Manual prerequisites are yours.** Before adding `owner-agreed` to a task whose Context lists a
   `Manual prerequisite (human): …` (OAuth client, Stripe keys, provisioned Postgres/Redis/MinIO, TURN),
   make sure it's satisfied — otherwise the worker builds a feature with nothing to connect to.

## Boundaries

- **Backend-centric.** Zones map to Go services. Frontend tasks are emitted only if the fleet builds
  frontend (a `frontend-conventions` skill + frontend zones in the map); otherwise triage the frontend
  separately.
- **Plans, never implements.** It writes issues; it never writes code, never approves, never runs the fleet.

## See also

- `SKILL.md` — the skill itself.
- `../work-cycle/SKILL.md` + `steps/` — the consuming fleet (`select`/`implement`/`review`/`address`).
- `../go-backend-conventions/SKILL.md`, `../new-service/SKILL.md`, `../new-resource/SKILL.md` — the canon
  feeder points each task at.
- `../../../.github/ISSUE_TEMPLATE/task.md` — the body format feeder must match.
