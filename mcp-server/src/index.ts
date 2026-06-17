import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { AgentClient } from "./client/agent.js";
import {
  inspectHandler,
  healthHandler,
  resourcesHandler,
  planRestartHandler,
} from "./tools/server.js";

const server = new McpServer({ name: "ai-server-agent", version: "1.0.0" });
const client = () => new AgentClient();

// ── Read tools ──

server.registerTool("server.inspect", {
  description: "查看服务器基本信息和资源使用情况。返回 CPU/Mem/Disk/OS/Kernel/Arch/Ports。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, inspectHandler);

server.registerTool("server.health", {
  description: "快速健康检查，返回健康状态和告警列表。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, healthHandler);

server.registerTool("server.resources", {
  description: "查看服务器详细资源使用（CPU/内存/磁盘百分比）。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, resourcesHandler);

server.registerTool("service.list", {
  description: "列出所有 systemd 服务及其运行状态。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, async (args) => {
  const d = await client().get("/api/v1/services");
  const svcs = d.services || [];
  return { content: [{ type: "text", text: `## Services (${svcs.length})\n| Service | Status |\n|---------|--------|\n${svcs.map((s: any) => `| ${s.name} | ${s.status} |`).join("\n")}` }], structuredContent: d };
});

server.registerTool("service.logs", {
  description: "查看 systemd 服务最近 N 行 journal 日志。",
  inputSchema: {
    server_id: z.string().describe("目标服务器 ID"),
    service_name: z.string().describe("systemd 服务名"),
    lines: z.number().optional().describe("行数，默认 50"),
  },
}, async (args) => {
  const d = await client().get(`/api/v1/services/${args.service_name}/logs?lines=${args.lines || 50}`);
  return { content: [{ type: "text", text: `## ${args.service_name} logs\n\`\`\`\n${(d.lines || []).join("\n")}\n\`\`\`` }], structuredContent: d };
});

// ── Docker tools ──

server.registerTool("docker.list", {
  description: "列出所有 Docker 容器及其状态（running/stopped/restarting）。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, async (args) => {
  const d = await client().get("/api/v1/docker/containers");
  const containers = d.containers || [];
  const text = [
    `## Docker Containers (${containers.length})`,
    ``,
    `| Name | Status | Image | Ports |`,
    `|------|--------|-------|-------|`,
    ...containers.map((c: any) => `| ${c.name} | ${c.status} | ${c.image} | ${(c.ports || []).join(", ") || "-"} |`),
  ].join("\n");
  return { content: [{ type: "text" as const, text }], structuredContent: d };
});

server.registerTool("docker.logs", {
  description: "查看 Docker 容器最近 N 行日志。",
  inputSchema: {
    server_id: z.string().describe("目标服务器 ID"),
    container_name: z.string().describe("容器名"),
    lines: z.number().optional().describe("行数，默认 50"),
  },
}, async (args) => {
  const d = await client().get(`/api/v1/docker/containers/${args.container_name}/logs?lines=${args.lines || 50}`);
  return { content: [{ type: "text", text: `## ${args.container_name} logs\n\`\`\`\n${(d.lines || []).join("\n")}\n\`\`\`` }], structuredContent: d };
});

server.registerTool("docker.plan_restart", {
  description: "生成重启 Docker 容器的计划。不会直接执行，返回 plan_id 后需调用 plan.apply。",
  inputSchema: { server_id: z.string(), container_name: z.string().describe("容器名，如 'myapp_web_1'") },
}, async (args) => {
  const d = await client().post("/api/v1/plans", {
    server_id: args.server_id, intent: `restart container ${args.container_name}`,
    actions: [{ type: "docker.restart", target: { kind: "docker_container", name: args.container_name } }],
  });
  return { content: [{ type: "text" as const, text: [
    `## Docker Restart Plan: ${args.container_name}`,
    ``, `| Field | Value |`, `|-------|-------|`,
    `| Plan ID | \`${d.plan_id}\` |`, `| Risk | **${d.risk}** |`,
    `| Requires Approval | ${d.requires_approval ? "✅ Yes" : "No"} |`,
    ``, `To execute: call \`plan.apply("${d.plan_id}")\``,
  ].join("\n") }], structuredContent: d };
});

// ── Write tools ──

server.registerTool("service.plan_restart", {
  description: "生成重启 systemd 服务的计划。不会直接执行，返回 plan_id 后需调用 plan.apply。",
  inputSchema: { server_id: z.string(), service_name: z.string() },
}, planRestartHandler);

server.registerTool("plan.apply", {
  description: "执行已创建的操作计划。传入 plan_id 执行对应操作。",
  inputSchema: { plan_id: z.string().describe("plan ID，从 service.plan_restart 等返回") },
}, async (args) => {
  const d = await client().post(`/api/v1/plans/${args.plan_id}/apply`, {});
  const results = d.results || [];
  const text = `## Plan ${args.plan_id}: ${d.status}\n${results.map((r: any, i: number) => `Step ${i + 1}: ${r.Success ? "✅" : "❌"} ${r.Stdout || r.Stderr || ""}`).join("\n")}`;
  return { content: [{ type: "text", text }], structuredContent: d };
});

// ── Audit ──

server.registerTool("audit.search", {
  description: "查询操作审计日志。可按 server_id、action_type、result 过滤。",
  inputSchema: {
    server_id: z.string().describe("服务器 ID"),
    action_type: z.string().optional().describe("操作类型"),
    result: z.string().optional().describe("结果：succeeded/failed"),
  },
}, async (args) => {
  const params = new URLSearchParams();
  params.set("server_id", args.server_id);
  if (args.action_type) params.set("action_type", args.action_type);
  if (args.result) params.set("result", args.result);
  const d = await client().get(`/api/v1/audit?${params}`);
  const events = d.events || [];
  const text = `## Audit Log (${d.total})\n| Time | Action | Target | Result |\n|------|--------|--------|--------|\n${events.map((e: any) => `| ${e.created_at} | ${e.action_type} | ${e.target} | ${e.result} |`).join("\n")}`;
  return { content: [{ type: "text", text }], structuredContent: d };
});

// ── Deploy tools ──

server.registerTool("app.plan_deploy", {
  description: "生成部署计划。接收 GitHub repo URL，检测运行时，评估风险，返回部署步骤。不会直接执行。",
  inputSchema: {
    server_id: z.string().describe("目标服务器 ID"),
    repo_url: z.string().describe("GitHub repo URL"),
    branch: z.string().optional().describe("分支，默认 main"),
    domain: z.string().optional().describe("域名"),
    app_name: z.string().optional().describe("应用名称"),
  },
}, async (args) => {
  const d = await client().post("/api/v1/deploy/plan", {
    server_id: args.server_id, repo_url: args.repo_url,
    branch: args.branch || "main", domain: args.domain || "", app_name: args.app_name || "",
  });
  return { content: [{ type: "text", text: `## Deploy Plan\n| Field | Value |\n|-------|-------|\n| Plan ID | \`${d.plan_id}\` |\n| App | ${d.app_name} |\n| Risk | **${d.risk}** |\n| Requires Approval | ${d.requires_approval ? "✅ Yes" : "No"} |\n| Steps | ${(d.steps || []).join(" → ")} |\n\n确认执行请调用 \`app.apply_deploy\`。` }], structuredContent: d };
});

server.registerTool("app.apply_deploy", {
  description: "执行部署计划。将 repo 克隆、构建、启动容器、运行健康检查、创建 release。",
  inputSchema: {
    plan_id: z.string().optional().describe("plan ID"),
    repo_url: z.string().describe("GitHub repo URL"),
    branch: z.string().optional().describe("分支"),
    app_name: z.string().optional().describe("应用名称"),
  },
}, async (args) => {
  const d = await client().post("/api/v1/deploy/apply", {
    plan_id: args.plan_id || "plan", repo_url: args.repo_url,
    branch: args.branch || "main", app_name: args.app_name || "",
  });
  const hc = d.healthcheck || {};
  const text = `## Deploy: ${d.status}\n| Field | Value |\n|-------|-------|\n| App | ${d.app_name} |\n| Release | ${d.release_id || "-"} |\n| Runtime | ${d.runtime || "-"} |\n| Healthcheck | ${hc.status || "-"} (${hc.latency_ms || 0}ms, HTTP ${hc.status_code || "-"}) |\n${d.error ? `| Error | ${d.error} |` : ""}`;
  return { content: [{ type: "text", text }], structuredContent: d };
});

server.registerTool("app.status", {
  description: "查询已部署应用的状态和当前 release 信息。",
  inputSchema: { app_name: z.string().describe("应用名称") },
}, async (args) => {
  const d = await client().get(`/api/v1/apps/${args.app_name}`);
  const r = d.release || {};
  return { content: [{ type: "text", text: `## ${args.app_name}\n| Field | Value |\n|-------|-------|\n| Release | ${r.release_id || "-"} |\n| Status | ${r.status || "-"} |\n| Commit | ${(r.commit || "").slice(0, 8)} |\n| Healthcheck | ${r.healthcheck_status || "-"} |` }], structuredContent: d };
});

server.registerTool("app.rollback", {
  description: "回滚应用到上一个版本。停止当前容器，checkout 旧 commit，重建并启动。",
  inputSchema: { app_name: z.string().describe("应用名称") },
}, async (args) => {
  const d = await client().post(`/api/v1/apps/${args.app_name}/rollback`, {});
  return { content: [{ type: "text", text: `## Rollback: ${d.status}\n${d.error ? `Error: ${d.error}` : `Restored to previous version.`}` }], structuredContent: d };
});

// ── Diagnosis ──

server.registerTool("diagnose.website", {
  description: "诊断网站不可访问的原因。检查端口监听、容器状态、健康检查。",
  inputSchema: {
    server_id: z.string().describe("服务器 ID"),
    domain: z.string().optional().describe("域名或 IP"),
    port: z.number().optional().describe("端口号，默认 80"),
  },
}, async (args) => {
  const port = args.port || 80;
  const reports: string[] = [];
  // 检查端口
  const d = await client().get("/api/v1/inspect");
  const ports = d.listening_ports || [];
  const found = ports.find((p: any) => p.port === port);
  reports.push(`- Port ${port}/${found?.protocol || "tcp"}: ${found ? `**${found.state}** (${found.process})` : "❌ NOT LISTENING"}`);
  // 检查 Docker 容器
  try {
    const ctn = await client().get("/api/v1/docker/containers");
    reports.push(`- Containers: ${(ctn.containers || []).length} running`);
  } catch { reports.push("- Containers: unable to check"); }
  return { content: [{ type: "text", text: `## Diagnose: ${args.domain || args.server_id}:${port}\n${reports.join("\n")}` }] };
});

// ── Start ──

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => { console.error("MCP Server fatal:", err); process.exit(1); });
