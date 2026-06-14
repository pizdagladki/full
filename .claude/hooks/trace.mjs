#!/usr/bin/env node
// PostToolUse hook: append ONE readable line per action to the fleet log.
// Always exits 0. Matcher "*" → logs Bash, Edit, Write, Agent, AND mcp__github__* calls.
import { appendFileSync, readFileSync } from "node:fs";

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8") || "{}");
} catch {
  process.exit(0);
}

try {
  const ti = input.tool_input || {};
  const tool = input.tool_name || "?";
  const agentType = input.agent_type ? `{${input.agent_type}} ` : "";

  // Most informative detail per tool.
  let detail =
    ti.command ||
    ti.file_path ||
    (ti.subagent_type ? `${ti.subagent_type}: ${ti.description || ""}` : "") ||
    ti.description ||
    ti.pattern ||
    ti.url ||
    ti.query ||
    "";

  // For github MCP tools, surface the issue/PR number (or a short owner/repo/title).
  if (!detail && tool.startsWith("mcp__github__")) {
    const n = ti.issue_number ?? ti.pull_number ?? ti.number;
    detail = n != null ? `#${n}` : ti.title || `${ti.owner || ""}/${ti.repo || ""}`;
  }

  const ts = new Date().toTimeString().slice(0, 8); // HH:MM:SS
  const who = process.env.FLEET_AGENT || "agent";
  let line = `${ts} [${who}] ${agentType}${tool} ${String(detail).replace(/\s+/g, " ").trim()}`;
  if (line.length > 140) line = line.slice(0, 140);

  appendFileSync(process.env.FLEET_LOG || "/tmp/fleet.log", line + "\n");
} catch {
  // never fail the tool
}
process.exit(0);
