#!/usr/bin/env node
// PostToolUse hook: report (never block) spelling issues via `typos` on the edited file.
// Reporting-only — ALWAYS exits 0. Silent no-op if `typos` isn't installed.
import { appendFileSync, readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8") || "{}");
} catch {
  process.exit(0);
}

try {
  const fp = input?.tool_input?.file_path;
  if (!fp) process.exit(0);

  const exts = [".go", ".md", ".ts", ".tsx", ".js", ".jsx", ".sql", ".yaml", ".yml", ".toml", ".txt"];
  if (!exts.some((e) => fp.endsWith(e))) process.exit(0);

  const bin = process.platform === "win32" ? "typos.exe" : "typos";
  const r = spawnSync(bin, [fp, "--format", "brief"], {
    cwd: process.env.CLAUDE_PROJECT_DIR || process.cwd(),
    encoding: "utf8",
  });

  // Binary missing → silent no-op (never fail a write because typos isn't installed).
  if (r.error) process.exit(0);

  const out = `${r.stdout || ""}${r.stderr || ""}`.trim();
  if (out) {
    const ts = new Date().toTimeString().slice(0, 8); // HH:MM:SS
    const who = process.env.FLEET_AGENT || "agent";
    const log = process.env.FLEET_LOG || "/tmp/fleet.log";
    for (const finding of out.split(/\r?\n/).filter(Boolean)) {
      const line = `${ts} [${who}] TYPOS ${fp}: ${finding}`.slice(0, 200);
      appendFileSync(log, line + "\n");
      process.stderr.write(line + "\n");
    }
  }
} catch {
  // never fail the tool
}
process.exit(0);
