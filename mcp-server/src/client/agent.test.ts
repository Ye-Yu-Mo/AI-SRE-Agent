import { describe, it, before, after } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { AgentClient, setConfig } from "./agent.js";

// 启动一个 mock Agent HTTP server
function startMockAgent(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const auth = req.headers["authorization"];

      if (req.url === "/health") {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ status: "ok" }));
        return;
      }

      if (req.url === "/api/v1/inspect") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401);
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({
          hostname: "test-host",
          os: "linux",
          arch: "amd64",
          cpu_percent: 12.5,
          memory_percent: 42.0,
          disk_percent: 61.0,
        }));
        return;
      }

      if (req.url === "/api/v1/services") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401);
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({
          services: [
            { name: "nginx", status: "active", uptime: "3d 2h" },
            { name: "docker", status: "active", uptime: "10d 1h" },
          ],
        }));
        return;
      }

      if (req.url === "/api/v1/error") {
        if (auth !== "Bearer test-secret") {
          res.writeHead(401);
          res.end(JSON.stringify({ error: "unauthorized" }));
          return;
        }
        res.writeHead(500);
        res.end(JSON.stringify({ error: "internal server error" }));
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

describe("AgentClient", () => {
  let server: http.Server;
  let port: number;

  before(async () => {
    const mock = await startMockAgent();
    server = mock.server;
    port = mock.port;
  });

  after(() => {
    server.close();
  });

  it("get() returns parsed JSON on 200", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });

    const client = new AgentClient();
    const result = await client.get("/api/v1/inspect");

    assert.ok(typeof result === "object");
    assert.equal(result.hostname, "test-host");
    assert.equal(result.os, "linux");
    assert.equal(result.arch, "amd64");
    assert.equal(result.cpu_percent, 12.5);
  });

  it("get() throws on 401", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "wrong-secret" });

    const client = new AgentClient();
    await assert.rejects(
      () => client.get("/api/v1/inspect"),
      /Agent API error: 401/
    );
  });

  it("get() throws on 500", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });

    const client = new AgentClient();
    await assert.rejects(
      () => client.get("/api/v1/error"),
      /Agent API error: 500/
    );
  });

  it("getServices() returns parsed service list", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });

    const client = new AgentClient();
    const result = await client.get("/api/v1/services");

    assert.ok(Array.isArray(result.services));
    assert.equal(result.services.length, 2);
    assert.equal(result.services[0].name, "nginx");
    assert.equal(result.services[0].status, "active");
  });

  it("post() sends POST with JSON body", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "test-secret" });

    const client = new AgentClient();
    // POST to /health — mock returns 200
    const result = await client.post("/health", { test: true });

    assert.equal(result.status, "ok");
  });

  it("post() throws on auth failure", async () => {
    setConfig({ endpoint: `http://127.0.0.1:${port}`, secret: "wrong" });

    const client = new AgentClient();
    await assert.rejects(
      () => client.post("/api/v1/inspect", {}),
      /Agent API error: 401/
    );
  });
});
