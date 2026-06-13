#!/usr/bin/env node
// PreToolUse hook: block any Edit/Write/MultiEdit whose target path is under .github/.
// .github/ is the human's zone (CODEOWNERS-protected); the autonomous flow must not be
// able to weaken its own gates. Deny = message on stderr + exit 2.
import { readFileSync } from "node:fs";

let input;
try {
  input = JSON.parse(readFileSync(0, "utf8") || "{}");
} catch {
  process.exit(0); // fail open on malformed input
}

const fp = input?.tool_input?.file_path;
if (typeof fp === "string") {
  const norm = fp.replace(/\\/g, "/");
  if (norm.includes("/.github/") || norm.startsWith(".github/")) {
    process.stderr.write(
      "Blocked: .github/ is the human's zone (CODEOWNERS-protected). " +
        "Agents must not edit CI or repo gates. Ask a human to make this change.\n"
    );
    process.exit(2);
  }
}
process.exit(0);
