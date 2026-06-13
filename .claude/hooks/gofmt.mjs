#!/usr/bin/env node
// PostToolUse hook: run `gofmt -w` on a .go file after Edit/Write/MultiEdit.
// Input: JSON on stdin (tool_input.file_path). Always exits 0 (non-blocking) so a
// formatting hiccup never fails a tool call.
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8") || "{}");
} catch {
  process.exit(0);
}

const fp = input?.tool_input?.file_path;
if (typeof fp === "string" && fp.endsWith(".go")) {
  const bin = process.platform === "win32" ? "gofmt.exe" : "gofmt";
  spawnSync(bin, ["-w", fp], { stdio: "inherit" });
}
process.exit(0);
