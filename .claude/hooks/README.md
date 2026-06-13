# .claude/hooks

Helper scripts for the hooks declared in `../settings.json`. Cross-platform (Node.js; `node` must be on PATH).
The hooks use exec form (`command: "node"`, `args: ["${CLAUDE_PROJECT_DIR}/.claude/hooks/<file>"]`) — the
Windows-recommended way to invoke a script per the Claude Code hooks docs.

- **`gofmt.mjs`** — PostToolUse (`Edit|Write|MultiEdit`). Runs `gofmt -w` on the edited file when it ends
  with `.go`. Non-blocking (always exits 0) — keeps diffs clean without ever failing a tool call.
- **`block-github.mjs`** — PreToolUse (`Edit|Write|MultiEdit`). Denies (exit 2 + stderr) any write whose
  path is under `.github/`. Defense-in-depth on top of CODEOWNERS so the autonomous flow can't weaken its
  own gates.

## Why there is no Stop hook
The build plan lists a `Stop → make lint` hook as optional. We deliberately omit it: a blocking Stop hook
fights the `work-cycle` design ("do one unit, then exit" — a blocking Stop prevents the headless process
from exiting cleanly), and lint is already enforced where it matters — CI is the real gate, and
`steps/implement.md` / `steps/address.md` run `make lint` before opening or updating a PR. Add one later
only if you want a local fast gate, and keep it non-blocking.

## Test the hooks
```bash
echo '{"tool_name":"Edit","tool_input":{"file_path":".github/workflows/ci.yml"}}' | node block-github.mjs; echo "exit=$?"   # -> exit=2 (blocked)
echo '{"tool_name":"Edit","tool_input":{"file_path":"services/x/main.go"}}'        | node block-github.mjs; echo "exit=$?"   # -> exit=0 (allowed)
echo '{"tool_name":"Write","tool_input":{"file_path":"foo.go"}}'                   | node gofmt.mjs;        echo "exit=$?"   # -> exit=0
```
Confirm the wiring interactively with `/hooks`.
