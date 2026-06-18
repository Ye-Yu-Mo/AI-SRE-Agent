import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { AgentClient } from "./client/agent.js";
import { addServer, removeServer } from "./client/agent.js";
import {
  inspectHandler,
  healthHandler,
  resourcesHandler,
  planRestartHandler,
  applyHandler,
  applyDeployHandler,
  diagnoseWebsiteHandler,
  serverListHandler,
} from "./tools/server.js";

const server = new McpServer({ name: "ai-server-agent", version: "1.0.0" });
const client = (serverId?: string) => new AgentClient(serverId);

// ── Read tools ──

server.registerTool("server.list", {
  description: "列出所有已配置的 Agent 服务器及其在线状态。",
  inputSchema: {},
}, serverListHandler);

// ── Server management ──

server.registerTool("server.add", {
  description: "添加一台 Agent 服务器到本地注册表（servers.json）。无需手动编辑配置文件。",
  inputSchema: {
    server_id: z.string().describe("服务器标识，如 srv_prod_01"),
    endpoint: z.string().describe("Agent URL，如 http://1.2.3.4:9090"),
    secret: z.string().describe("Agent secret（安装时打印的密钥）"),
  },
}, async (args) => {
  addServer(args.server_id, args.endpoint, args.secret);
  return { content: [{ type: "text" as const, text: `## Server Added\n| Field | Value |\n|-------|-------|\n| ID | ${args.server_id} |\n| Endpoint | ${args.endpoint} |\n\nRestart Claude Code or reconnect MCP to apply.` }] };
});

server.registerTool("server.remove", {
  description: "从本地注册表中删除一台 Agent 服务器。",
  inputSchema: {
    server_id: z.string().describe("要删除的服务器标识"),
  },
}, async (args) => {
  const ok = removeServer(args.server_id);
  return { content: [{ type: "text" as const, text: ok ? `## Removed\nServer \`${args.server_id}\` removed from registry.` : `## Not Found\nServer \`${args.server_id}\` not in registry.` }] };
});

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
  const d = await client(args.server_id).get("/api/v1/services");
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
  const d = await client(args.server_id).get(`/api/v1/services/${args.service_name}/logs?lines=${args.lines || 50}`);
  return { content: [{ type: "text", text: `## ${args.service_name} logs\n\`\`\`\n${(d.lines || []).join("\n")}\n\`\`\`` }], structuredContent: d };
});

// ── Docker tools ──

server.registerTool("docker.list", {
  description: "列出所有 Docker 容器及其状态（running/stopped/restarting）。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, async (args) => {
  const d = await client(args.server_id).get("/api/v1/docker/containers");
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
  const d = await client(args.server_id).get(`/api/v1/docker/containers/${args.container_name}/logs?lines=${args.lines || 50}`);
  return { content: [{ type: "text", text: `## ${args.container_name} logs\n\`\`\`\n${(d.lines || []).join("\n")}\n\`\`\`` }], structuredContent: d };
});

server.registerTool("docker.plan_restart", {
  description: "生成重启 Docker 容器的计划。不会直接执行，返回 plan_id 后需调用 plan.apply。",
  inputSchema: { server_id: z.string(), container_name: z.string().describe("容器名，如 'myapp_web_1'") },
}, async (args) => {
  const d = await client(args.server_id).post("/api/v1/plans", {
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
  description: "执行已创建的操作计划。高风险操作首次调用会返回需确认卡片——必须先把卡片展示给用户、获得明确确认后，才能带 confirm=true 重试。不得在用户未确认时自行带 confirm。",
  inputSchema: {
    plan_id: z.string().describe("plan ID，从 service.plan_restart 等返回"),
    confirm: z.boolean().optional().describe("用户已明确确认高风险操作时传 true"),
    server_id: z.string().optional().describe("目标服务器 ID"),
  },
}, applyHandler);

// ── Audit ──

server.registerTool("audit.search", {
  description: "查询操作审计日志。可按 server_id、action_type、result 过滤。",
  inputSchema: {
    server_id: z.string().optional().describe("服务器 ID（可选，不传则查当前路由的 Agent）"),
    action_type: z.string().optional().describe("操作类型"),
    result: z.string().optional().describe("结果：succeeded/failed"),
  },
}, async (args) => {
  const params = new URLSearchParams();
  if (args.server_id) params.set("server_id", args.server_id);
  if (args.action_type) params.set("action_type", args.action_type);
  if (args.result) params.set("result", args.result);
  const d = await client(args.server_id).get(`/api/v1/audit?${params}`);
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
  const d = await client(args.server_id).post("/api/v1/deploy/plan", {
    server_id: args.server_id, repo_url: args.repo_url,
    branch: args.branch || "main", domain: args.domain || "", app_name: args.app_name || "",
  });
  return { content: [{ type: "text", text: `## Deploy Plan\n| Field | Value |\n|-------|-------|\n| Plan ID | \`${d.plan_id}\` |\n| App | ${d.app_name} |\n| Risk | **${d.risk}** |\n| Requires Approval | ${d.requires_approval ? "✅ Yes" : "No"} |\n| Steps | ${(d.steps || []).join(" → ")} |\n\n确认执行请调用 \`app.apply_deploy\`。` }], structuredContent: d };
});

server.registerTool("app.apply_deploy", {
  description: "执行部署：clone → build → up → healthcheck → release。高危 compose 配置触发 409 拦截。带 domain 参数自动配 Caddy 反向代理 + HTTPS。",
  inputSchema: {
    plan_id: z.string().optional().describe("plan ID"),
    server_id: z.string().optional().describe("目标服务器 ID"),
    repo_url: z.string().describe("GitHub repo URL"),
    branch: z.string().optional().describe("分支"),
    app_name: z.string().optional().describe("应用名称"),
    domain: z.string().optional().describe("域名"),
    confirm: z.boolean().optional().describe("用户已审阅 supply chain 风险并明确确认时传 true"),
  },
}, applyDeployHandler);

server.registerTool("app.status", {
  description: "查询已部署应用的状态和当前 release 信息。",
  inputSchema: { app_name: z.string().describe("应用名称"), server_id: z.string().optional().describe("目标服务器 ID") },
}, async (args) => {
  const d = await client(args.server_id).get(`/api/v1/apps/${args.app_name}`);
  const r = d.release || {};
  return { content: [{ type: "text", text: `## ${args.app_name}\n| Field | Value |\n|-------|-------|\n| Release | ${r.release_id || "-"} |\n| Status | ${r.status || "-"} |\n| Commit | ${(r.commit || "").slice(0, 8)} |\n| Healthcheck | ${r.healthcheck_status || "-"} |` }], structuredContent: d };
});

server.registerTool("app.rollback", {
  description: "回滚应用到上一个版本。停止当前容器，checkout 旧 commit，重建并启动。",
  inputSchema: { app_name: z.string().describe("应用名称"), server_id: z.string().optional().describe("目标服务器 ID") },
}, async (args) => {
  const d = await client(args.server_id).post(`/api/v1/apps/${args.app_name}/rollback`, {});
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
}, diagnoseWebsiteHandler);

// ── Graph ──

server.registerTool("server.graph", {
  description: "查看服务器 State Graph — 应用/容器/端口/反向代理的拓扑依赖关系。用于理解'谁依赖谁'、'哪个组件坏了会影响什么'。",
  inputSchema: { server_id: z.string().describe("目标服务器 ID") },
}, async (args) => {
  const g = await client(args.server_id).get("/api/v1/graph");
  const nodes = g.nodes || [];
  const edges = g.edges || [];
  const text = [
    `## State Graph`,
    ``,
    `### Nodes (${nodes.length})`,
    `| ID | Kind | Name | Status |`,
    `|----|------|------|--------|`,
    ...nodes.map((n: any) => `| ${n.id} | ${n.kind} | ${n.name} | ${n.status || "-"} |`),
    ``,
    `### Edges (${edges.length})`,
    `| From | Type | To |`,
    `|------|------|----|`,
    ...edges.map((e: any) => `| ${e.from} | ${e.type} | ${e.to} |`),
  ].join("\n");
  return { content: [{ type: "text" as const, text }], structuredContent: g };
});

// ── File & Command tools ──

server.registerTool("agent.update", {
  description: "更新 Agent 到最新版本（或指定版本）。Agent 自动下载最新 GitHub Release binary、替换、重启、验证。失败自动回滚。",
  inputSchema: {
    server_id: z.string().optional().describe("目标服务器 ID（不传则更新所有在线服务器）"),
    version: z.string().optional().describe("指定版本号如 v0.8.0（不传则用最新）"),
  },
}, async (args) => {
  const d = await client(args.server_id).post("/api/v1/agent/update", {
    version: args.version || "",
  }, 120_000);
  return { content: [{ type: "text" as const, text: `## Agent Update: ${d.status}\n${d.error ? `Error: ${d.error}` : "Updated successfully."}` }], structuredContent: d };
});

server.registerTool("file.write", {
  description: "上传文件到 Agent 服务器。content 为 base64 编码，path 禁止写入 /etc /boot /sys /proc。",
  inputSchema: {
    server_id: z.string().optional().describe("目标服务器 ID"),
    path: z.string().describe("目标文件路径，如 /var/lib/myapp/config.yaml"),
    content: z.string().describe("文件内容（base64 编码）"),
  },
}, async (args) => {
  const d = await client(args.server_id).post("/api/v1/files/write", {
    path: args.path, content: args.content,
  }, 30_000);
  return { content: [{ type: "text" as const, text: `## File Written\n| Field | Value |\n|-------|-------|\n| Path | \`${d.path}\` |\n| Size | ${d.size}B |\n| Status | ${d.status} |` }], structuredContent: d };
});

server.registerTool("command.run", {
  description: "在 Agent 服务器上执行 shell 命令。必须把命令展示给用户并获得明确确认后，才能带 confirm=true 执行。不得在用户未确认时自行带 confirm。",
  inputSchema: {
    server_id: z.string().optional().describe("目标服务器 ID"),
    command: z.string().describe("要执行的命令，如 'bash scalping/start.sh start'"),
    work_dir: z.string().optional().describe("工作目录"),
    timeout: z.number().optional().describe("超时秒数，默认 30，最大 300"),
    confirm: z.boolean().optional().describe("用户已审阅命令并明确确认时传 true"),
  },
}, async (args) => {
  // 审批闸门：不带 confirm 时返回确认卡片
  if (!args.confirm) {
    return { content: [{ type: "text" as const, text: [
      "## ⚠️ 命令执行需要确认",
      "",
      "**将要执行的命令：**",
      "```",
      args.command,
      "```",
      args.work_dir ? `**工作目录：** ${args.work_dir}` : "",
      "**超时：** " + (args.timeout || 30) + "s",
      "",
      "请将本卡片展示给用户，**不要自行决定**。",
      "仅当用户明确表示确认后，再调用 `command.run` 并传入 `confirm: true`。",
    ].join("\n") }] };
  }
  const t = args.timeout ? Math.min(args.timeout, 300) : 30;
  const d = await client(args.server_id).post("/api/v1/commands/run", {
    command: args.command, work_dir: args.work_dir || "", timeout: t,
  }, (t + 5) * 1000);
  return { content: [{ type: "text" as const, text: `## Command: ${d.status}\n\`\`\`\n${(d.stdout || "").slice(0, 2000)}\n\`\`\`\nExit code: ${d.exit_code}` }], structuredContent: d };
});

// ── Start ──

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => { console.error("MCP Server fatal:", err); process.exit(1); });
