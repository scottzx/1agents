#!/usr/bin/env node
// capture-acp.mjs — capture the full ACP JSON-RPC stdio stream of an agent
// and answer: when the main agent spawns a SUBAGENT, how does the adapter mark
// subagent-attributed chunks vs top-level output?
//
// Usage:
//   node scripts/capture-acp.mjs [--agent "<cmd>"] [--cwd <dir>] [--prompt <text>] [--out <file>] [--timeout-ms <n>]
//
// Defaults:
//   --agent  npx -y @agentclientprotocol/claude-agent-acp@0.60.0   (Claude Code, same as the 1agents registry)
//   --cwd    a scratch dir with one sample file (so the subagent has minimal work)
//   --prompt  asks the main agent to spawn a Task-tool subagent and report back
//
// Output: a JSONL file with three kinds of records:
//   {"ts","dir":"C->A"|"A->C","line":"<raw JSON-RPC line>"}     — every raw wire line
//   {"ts","dir":"update","update":{...}}                          — every structured session/update (full _meta)
//   {"ts","dir":"note","note":"..."}                              — lifecycle notes
// plus a condensed summary table printed to stderr at the end.

import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { PassThrough, Readable } from "node:stream";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(__dirname, "..");
const SDK_ACP = path.join(
  REPO_ROOT,
  "modules/1acp/node_modules/@agentclientprotocol/sdk/dist/acp.js",
);
const SDK_STREAM = path.join(
  REPO_ROOT,
  "modules/1acp/node_modules/@agentclientprotocol/sdk/dist/stream.js",
);

const { ClientSideConnection } = await import(SDK_ACP);
const { ndJsonStream } = await import(SDK_STREAM);
const { PROTOCOL_VERSION } = await import(SDK_ACP);

// ---------------------------------------------------------------- args

function argValue(argv, flag, fallback) {
  const i = argv.indexOf(flag);
  return i >= 0 && argv[i + 1] !== undefined ? argv[i + 1] : fallback;
}

const argv = process.argv.slice(2);
const agentCommand = argValue(argv, "--agent", "npx -y @agentclientprotocol/claude-agent-acp@0.60.0");
const cwdArg = argValue(argv, "--cwd", "");
const promptArg = argValue(argv, "--prompt", "");
const timeoutMs = Number(argValue(argv, "--timeout-ms", String(3 * 60 * 1000)));
const outFile = argValue(
  argv,
  "--out",
  path.join(REPO_ROOT, "scripts/acp-capture", `${Date.now()}-capture.jsonl`),
);

// A tiny scratch workspace so the subagent has minimal, predictable work.
const scratchDir = cwdArg
  ? path.resolve(cwdArg)
  : fs.mkdtempSync(path.join(os.tmpdir(), "acp-capture-"));
if (!cwdArg) {
  fs.writeFileSync(
    path.join(scratchDir, "sample.txt"),
    "package name: sample-package\ndescription: a tiny scratch package used to observe subagent output attribution\n",
  );
}

const defaultPrompt =
  "Use the Task tool to spawn a subagent to review the files in this directory and report back a short summary of what it found. Then give me a one-line summary of the subagent's report.";
const promptText = promptArg || defaultPrompt;

fs.mkdirSync(path.dirname(outFile), { recursive: true });
const capture = fs.createWriteStream(outFile, { flags: "a" });
const note = (text) => {
  const record = JSON.stringify({ ts: new Date().toISOString(), dir: "note", note: text });
  capture.write(record + "\n");
  console.error(`[capture] ${text}`);
};

// ---------------------------------------------------------------- line tee

function makeLineSplitter(onLine) {
  let buf = "";
  return (chunk) => {
    buf += chunk.toString();
    let idx;
    while ((idx = buf.indexOf("\n")) >= 0) {
      const line = buf.slice(0, idx);
      buf = buf.slice(idx + 1);
      const trimmed = line.trim();
      if (trimmed) onLine(trimmed);
    }
  };
}

// ---------------------------------------------------------------- spawn + transport

note(`spawning: ${agentCommand}`);
note(`cwd: ${scratchDir}`);
note(`out: ${outFile}`);

const child = spawn(agentCommand, { shell: true, cwd: scratchDir, stdio: ["pipe", "pipe", "pipe"] });
let stderrTail = "";
child.stderr.on("data", (c) => {
  stderrTail = (stderrTail + c.toString()).slice(-4000);
});

const structuredUpdates = [];
const rawUpdates = [];

const teeIncoming = makeLineSplitter((line) => {
  rawUpdates.push({ dir: "A->C", line });
  capture.write(JSON.stringify({ ts: new Date().toISOString(), dir: "A->C", line }) + "\n");
});
child.stdout.on("data", teeIncoming);

// Outgoing: log each serialized message, then forward bytes to the agent.
const outgoing = new WritableStream({
  write(chunk) {
    const text = new TextDecoder().decode(chunk);
    for (const l of text.split("\n")) {
      const trimmed = l.trim();
      if (trimmed) {
        rawUpdates.push({ dir: "C->A", line: trimmed });
        capture.write(JSON.stringify({ ts: new Date().toISOString(), dir: "C->A", line: trimmed }) + "\n");
      }
    }
    child.stdin.write(chunk);
  },
});

const passthrough = new PassThrough();
child.stdout.pipe(passthrough);
const stream = ndJsonStream(outgoing, Readable.toWeb(passthrough));

// ---------------------------------------------------------------- client handlers

const client = {
  async requestPermission(params) {
    const options = params?.options ?? [];
    if (options.length === 0) {
      return { outcome: { outcome: "cancelled" } };
    }
    const allow = options.find(
      (o) => o.kind === "allow_once" || o.kind === "allow_always",
    );
    return { outcome: { outcome: "selected", optionId: (allow ?? options[0]).optionId } };
  },
  async sessionUpdate(params) {
    structuredUpdates.push(params);
    capture.write(
      JSON.stringify({ ts: new Date().toISOString(), dir: "update", update: params }) + "\n",
    );
  },
  async readTextFile(params) {
    const p = path.isAbsolute(params.path) ? params.path : path.join(scratchDir, params.path);
    return { content: fs.readFileSync(p, "utf8") };
  },
  async writeTextFile() {
    return {};
  },
};

// Unknown client methods must not crash the SDK — no-op fallback.
const clientProxy = new Proxy(client, {
  get(target, prop) {
    if (prop in target) return target[prop];
    return async () => {};
  },
});

// ---------------------------------------------------------------- run

let done = false;
const summarize = () => {
  const kinds = new Map();
  for (const u of structuredUpdates) {
    const tag = u?.update?.sessionUpdate ?? "?";
    kinds.set(tag, (kinds.get(tag) ?? 0) + 1);
  }
  console.error("\n===== SUMMARY =====");
  console.error(`updates by sessionUpdate type:`);
  for (const [k, n] of [...kinds.entries()].sort((a, b) => b[1] - a[1])) {
    console.error(`  ${k}: ${n}`);
  }

  const inner = structuredUpdates.map((u) => u?.update ?? {});
  const subagentChunks = inner.filter(
    (u) => u?._meta?.claudeCode?.parentToolUseId,
  );
  const textChunks = inner.filter(
    (u) => u?.sessionUpdate === "agent_message_chunk" || u?.sessionUpdate === "agent_thought_chunk",
  );
  const msgIds = new Set(textChunks.map((u) => u?.messageId).filter(Boolean));
  const toolCalls = inner.filter((u) => u?.sessionUpdate === "tool_call");
  const withParent = toolCalls.filter((u) => u?._meta?.claudeCode?.parentToolUseId);
  console.error(`\ntext chunks: ${textChunks.length} across ${msgIds.size} messageId(s)`);
  console.error(`chunks carrying _meta.claudeCode.parentToolUseId: ${subagentChunks.length}`);
  console.error(`tool_call updates: ${toolCalls.length} (${withParent.length} attributed to a subagent via parentToolUseId)`);
  for (const u of toolCalls.slice(0, 30)) {
    const c = u?.toolCall ?? u?.content ?? {};
    console.error(
      `  tool: ${u?.title ?? "?"} id=${u?.toolCallId ?? "?"} parent=${u?._meta?.claudeCode?.parentToolUseId ?? "—"}`,
    );
  }
  console.error(`\nraw capture: ${outFile}`);
};
const shutdown = async (code) => {
  if (done) return;
  done = true;
  try {
    child.kill("SIGTERM");
  } catch {
    /* ignore */
  }
  await new Promise((r) => setTimeout(r, 200));
  await new Promise((resolve) => capture.end(resolve));
  summarize();
  process.exit(code ?? 0);
};
process.on("SIGINT", () => void shutdown(0));
process.on("SIGTERM", () => void shutdown(0));
setTimeout(() => {
  note("TIMEOUT: killing agent and flushing capture");
  void shutdown(1);
}, timeoutMs);

const connection = new ClientSideConnection(() => clientProxy, stream);

try {
  const init = await connection.initialize({
    protocolVersion: PROTOCOL_VERSION,
    clientCapabilities: { fs: { readTextFile: true, writeTextFile: true } },
    clientInfo: { name: "acp-capture", version: "1.0.0" },
  });
  note(`initialize ok (protocolVersion=${init.protocolVersion})`);

  const created = await connection.newSession({ cwd: scratchDir, mcpServers: [] });
  const sessionId = created.sessionId;
  note(`session/new ok (sessionId=${sessionId})`);

  const promptStart = Date.now();
  const response = await connection.prompt({
    sessionId,
    prompt: [{ type: "text", text: promptText }],
  });
  note(`prompt done in ${((Date.now() - promptStart) / 1000).toFixed(1)}s (stopReason=${response.stopReason})`);

  await connection.closeSession({ sessionId }).catch(() => {});
  note("session/close ok");
} catch (err) {
  note(`ERROR: ${err?.message ?? String(err)}`);
  const detail = stderrTail.trim();
  if (detail) {
    note(`agent stderr tail:\n${detail}`);
  }
} finally {
  try {
    connection.close?.();
  } catch {
    /* best-effort teardown */
  }
  await new Promise((r) => setTimeout(r, 300));
  await shutdown(0);
}

