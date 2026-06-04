import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = fileURLToPath(new URL(".", import.meta.url));

function sendRPC(
  proc: ReturnType<typeof spawn>,
  method: string,
  params?: unknown
): Promise<unknown> {
  const id = Math.random().toString(36).slice(2);
  const msg = JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n";

  return new Promise((resolve, reject) => {
    const onData = (data: Buffer) => {
      const lines = data.toString().split("\n").filter(Boolean);
      for (const line of lines) {
        try {
          const r = JSON.parse(line);
          if (r.id === id) {
            proc.stdout!.removeListener("data", onData);
            resolve(r);
          }
        } catch {
          // partial line, ignore
        }
      }
    };
    proc.stdout!.on("data", onData);
    proc.stdin!.write(msg);

    setTimeout(() => {
      proc.stdout!.removeListener("data", onData);
      reject(new Error(`RPC timeout: ${method}`));
    }, 10_000);
  });
}

describe("MCP Server — integration (real agent)", () => {
  it("initialize + tools/list returns registered tools", async () => {
    const proc = spawn("node", [join(__dirname, "../dist/index.js")], {
      stdio: ["pipe", "pipe", "pipe"],
      env: {
        ...process.env,
        AGENT_ENDPOINT: "http://47.92.153.60:9090",
        AGENT_SECRET: "it-test-secret-1780557281",
      },
    });

    try {
      // 1. Initialize
      const initResp = (await sendRPC(proc, "initialize", {
        protocolVersion: "2025-11-25",
        capabilities: {},
        clientInfo: { name: "test", version: "1.0" },
      })) as any;

      assert.ok(initResp.result, `init failed: ${JSON.stringify(initResp)}`);
      assert.equal(initResp.result.serverInfo.name, "ai-server-agent");
      assert.ok(initResp.result.capabilities.tools, "should have tools capability");

      // 2. List tools
      const listResp = (await sendRPC(proc, "tools/list", {})) as any;
      assert.ok(listResp.result.tools, `tools/list failed: ${JSON.stringify(listResp)}`);
      const tools = listResp.result.tools as Array<{ name: string }>;
      assert.ok(tools.length >= 4, `expected at least 4 tools, got ${tools.length}`);

      const names = tools.map((t) => t.name);
      assert.ok(names.includes("server.inspect"), "missing server.inspect");
      assert.ok(names.includes("server.health"), "missing server.health");
      assert.ok(names.includes("server.resources"), "missing server.resources");
      assert.ok(names.includes("service.plan_restart"), "missing service.plan_restart");

      console.log(`Discovered ${tools.length} tools: ${names.join(", ")}`);

      // 3. Call server.inspect
      const callResp = (await sendRPC(proc, "tools/call", {
        name: "server.inspect",
        arguments: { server_id: "srv_remote_01" },
      })) as any;

      assert.ok(callResp.result, `tools/call failed: ${JSON.stringify(callResp)}`);
      const content = callResp.result.content;
      assert.ok(content.length > 0, "inspect response has content");
      const text = content[0].text as string;
      assert.ok(text.includes("##"), `expected markdown, got: ${text?.slice(0, 100)}`);
      console.log(`inspect response (first 200 chars): ${text?.slice(0, 200)}`);
    } finally {
      proc.kill();
    }
  });
});
