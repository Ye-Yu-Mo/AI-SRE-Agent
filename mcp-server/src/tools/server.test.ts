import { describe, it, before, after } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { setConfig } from "../client/agent.js";

// 直接测试 tool handler 的输出格式（不通过 MCP SDK）
// 这些 handler 在 index.ts 中被 registerTool 使用

function startMockAgent(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const auth = req.headers["authorization"];

      if (req.url === "/api/v1/inspect") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({
          hostname: "prod-01",
          os: "ubuntu",
          os_version: "22.04",
          kernel: "5.15.0",
          arch: "x86_64",
          cpu_percent: 23.5,
          memory_total: 17179869184,
          memory_used: 7215545057,
          memory_percent: 42.0,
          disk_total: 536870912000,
          disk_used: 327491256320,
          disk_percent: 61.0,
        }));
        return;
      }

      if (req.url === "/api/v1/health") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({
          status: "healthy",
          warnings: ["disk usage 61%"],
        }));
        return;
      }

      if (req.method === "POST" && req.url === "/api/v1/plans") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        let body = "";
        req.on("data", (d) => (body += d));
        req.on("end", () => {
          res.writeHead(201, { "Content-Type": "application/json" });
          res.end(JSON.stringify({
            plan_id: "plan_abc123",
            status: "pending",
            risk: "medium",
            requires_approval: true,
            steps: [{ step: 1, action: { type: "service.restart", target: { kind: "systemd_service", name: "nginx" } } }],
          }));
        });
        return;
      }

      // apply: plan_high requires approval (409), plan_low succeeds (200)
      if (req.method === "POST" && req.url?.startsWith("/api/v1/plans/")) {
        let body = "";
        req.on("data", (d) => (body += d));
        req.on("end", () => {
          const parsed = JSON.parse(body || "{}");
          const planId = req.url!.split("/")[4];
          if (planId === "plan_high" && !parsed.approve) {
            res.writeHead(409, { "Content-Type": "application/json" });
            res.end(JSON.stringify({ error: "approval required", plan_id: planId }));
          } else {
            res.writeHead(200, { "Content-Type": "application/json" });
            res.end(JSON.stringify({ plan_id: planId, status: "succeeded", results: [] }));
          }
        });
        return;
      }

      res.writeHead(404);
      res.end("not found");
    });

    server.listen(0, "127.0.0.1", () => {
      const addr = server.address() as { port: number };
      resolve({ server, port: addr.port });
    });
  });
}

describe("Tool handlers — output format", () => {
  let server: http.Server;
  let port: number;

  before(async () => {
    const mock = await startMockAgent();
    server = mock.server;
    port = mock.port;
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });
  });

  after(() => {
    server.close();
  });

  it("inspect handler returns Markdown containing key fields", async () => {
    // 动态导入 handler（避免顶层 import 依赖循环）
    const { inspectHandler } = await import("./server.js");
    const result = await inspectHandler({ server_id: "srv_01" });

    assert.ok(result.content.length > 0);
    const text = result.content[0].text;
    assert.ok(typeof text === "string");
    // Markdown 格式
    assert.ok(text.includes("##"), `expected ## headers, got: ${text.slice(0, 100)}`);
    assert.ok(text.includes("prod-01"), `expected hostname, got: ${text.slice(0, 100)}`);
    assert.ok(text.includes("23.5"), `expected cpu, got: ${text.slice(0, 100)}`);
    assert.ok(text.includes("42%"), `expected memory percent, got: ${text.slice(0, 100)}`);
  });

  it("health handler returns status and warnings", async () => {
    const { healthHandler } = await import("./server.js");
    const result = await healthHandler({ server_id: "srv_01" });

    const text = result.content[0].text;
    assert.ok(text.includes("healthy"));
    assert.ok(text.includes("61%"));
  });

  it("plan_restart handler returns plan card with plan_id", async () => {
    const { planRestartHandler } = await import("./server.js");
    const result = await planRestartHandler({ server_id: "srv_01", service_name: "nginx" });

    const text = result.content[0].text;
    assert.ok(text.includes("plan_abc123"), `expected plan_id in: ${text}`);
    assert.ok(text.includes("medium"), `expected risk level in: ${text}`);
    assert.ok(text.includes("nginx"), `expected service name in: ${text}`);
  });

  it("handler returns isError on agent communication failure", async () => {
    // 用错误的 endpoint 模拟连接失败
    setConfig({ endpoint: "http://127.0.0.1:19999", secret: "bad" });
    const { inspectHandler } = await import("./server.js");

    await assert.rejects(
      () => inspectHandler({ server_id: "srv_01" }),
      /Agent API error/
    );
  });
});

describe("plan.apply — approval gate", () => {
  let server: http.Server;
  let port: number;

  before(async () => {
    const mock = await startMockAgent();
    server = mock.server;
    port = mock.port;
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });
  });

  after(() => {
    server.close();
  });

  // M2: 撞 409 不抛异常，返回知情确认卡片，且指示 AI 停下问用户。
  it("returns confirmation card on 409, does not throw", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });
    const { applyHandler } = await import("./server.js");

    const result = await applyHandler({ plan_id: "plan_high" });
    const text = result.content[0].text;
    assert.ok(text.includes("plan_high"), `expected plan_id in card: ${text}`);
    // 卡片必须包含"需确认/确认"语义，且不能是裸异常
    assert.ok(/确认|approval|confirm/i.test(text), `expected approval wording: ${text}`);
  });

  // M2: confirm=true → 转发 approve，正常执行不再 409。
  it("with confirm=true passes the gate", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });
    const { applyHandler } = await import("./server.js");

    const result = await applyHandler({ plan_id: "plan_high", confirm: true });
    const text = result.content[0].text;
    assert.ok(text.includes("succeeded"), `expected success: ${text}`);
  });

  // M2: 真错误（500）不被 409 分支吞掉，照常抛出。
  it("rethrows non-409 errors", async () => {
    setConfig({ endpoint: "http://127.0.0.1:19999", secret: "bad" });
    const { applyHandler } = await import("./server.js");

    await assert.rejects(() => applyHandler({ plan_id: "plan_x" }), /Agent API error/);
  });
});
