import { AgentClient, AgentError, listEndpoints } from "../client/agent.js";

function client(serverId?: string): AgentClient {
  return new AgentClient(serverId);
}

export async function inspectHandler(args: { server_id: string }) {
  const data = await client(args.server_id).get("/api/v1/inspect");

  const text = [
    `## Server: ${data.hostname}`,
    ``,
    `| Field | Value |`,
    `|-------|-------|`,
    `| OS | ${data.os} ${data.os_version || ""} |`,
    `| Kernel | ${data.kernel || "N/A"} |`,
    `| Arch | ${data.arch || "N/A"} |`,
    `| CPU | ${data.cpu_percent}% |`,
    `| Memory | ${data.memory_percent}% (${formatBytes(data.memory_used)} / ${formatBytes(data.memory_total)}) |`,
    `| Disk | ${data.disk_percent}% (${formatBytes(data.disk_used)} / ${formatBytes(data.disk_total)}) |`,
  ].join("\n");

  return {
    content: [{ type: "text" as const, text }],
    structuredContent: data,
  };
}

export async function healthHandler(args: { server_id: string }) {
  const data = await client(args.server_id).get("/api/v1/health");

  const warnings = data.warnings?.length
    ? data.warnings.map((w: string) => `- ⚠️ ${w}`).join("\n")
    : "- None";

  const text = [
    `## Health: ${data.status}`,
    ``,
    `### Warnings`,
    warnings,
  ].join("\n");

  return {
    content: [{ type: "text" as const, text }],
    structuredContent: data,
  };
}

export async function resourcesHandler(args: { server_id: string }) {
  const data = await client(args.server_id).get("/api/v1/resources");

  const text = [
    `## Resources`,
    ``,
    `| Resource | Usage |`,
    `|----------|-------|`,
    `| CPU | ${data.cpu_percent ?? "N/A"}% |`,
    `| Memory | ${data.memory_percent ?? "N/A"}% |`,
    `| Disk | ${data.disk_percent ?? "N/A"}% |`,
  ].join("\n");

  return {
    content: [{ type: "text" as const, text }],
    structuredContent: data,
  };
}

export async function planRestartHandler(args: { server_id: string; service_name: string }) {
  const data = await client(args.server_id).post("/api/v1/plans", {
    server_id: args.server_id,
    intent: `restart ${args.service_name}`,
    actions: [
      {
        type: "service.restart",
        target: { kind: "systemd_service", name: args.service_name },
      },
    ],
  });

  const text = [
    `## Restart Plan: ${args.service_name}`,
    ``,
    `| Field | Value |`,
    `|-------|-------|`,
    `| Plan ID | \`${data.plan_id}\` |`,
    `| Risk | **${data.risk}** |`,
    `| Requires Approval | ${data.requires_approval ? "✅ Yes" : "No"} |`,
    `| Steps | ${data.steps?.length || 0} |`,
    ``,
    `To execute: call \`plan.apply("${data.plan_id}")\``,
  ].join("\n");

  return {
    content: [{ type: "text" as const, text }],
    structuredContent: data,
  };
}

// M2: applyHandler — 接住 409（需审批），返回知情确认卡片，不抛异常。
// 其他错误（500/404）照常上抛。
export async function applyHandler(args: { plan_id: string; confirm?: boolean; server_id?: string }) {
  try {
    const d = await client(args.server_id).post(`/api/v1/plans/${args.plan_id}/apply`, {
      approve: args.confirm === true,
    });
    const results = d.results || [];
    const text = `## Plan ${args.plan_id}: ${d.status}\n${results.map((r: any, i: number) => `Step ${i + 1}: ${r.Success ? "✅" : "❌"} ${r.Stdout || r.Stderr || ""}`).join("\n")}`;
    return { content: [{ type: "text" as const, text }], structuredContent: d };
  } catch (e) {
    if (e instanceof AgentError && e.status === 409) {
      const text = [
        `## ⚠️ 此操作需要确认`,
        ``,
        `Plan \`${args.plan_id}\` 风险等级高，已被安全闸门拦截。`,
        ``,
        `**请将本卡片展示给用户，不要自行决定。**`,
        `仅当用户明确表示确认后，再调用 \`plan.apply\` 并传入 \`confirm: true\`。`,
      ].join("\n");
      return { content: [{ type: "text" as const, text }] };
    }
    throw e;
  }
}

export async function diagnoseWebsiteHandler(args: {
  server_id: string;
  domain?: string;
  port?: number;
}) {
  const port = args.port || 80;
  const lines: string[] = [];

  // 端口状态
  const d = await client(args.server_id).get("/api/v1/inspect");
  const ports: any[] = d.listening_ports || [];
  const found = ports.find((p: any) => p.port === port);
  lines.push(`**Port ${port}:** ${found ? `✅ LISTEN (${found.process || "-"})` : "❌ NOT LISTENING"}`);

  // 容器列表 — 每个容器独立一行，不再只报数量
  let containers: any[] = [];
  try {
    const ctn = await client(args.server_id).get("/api/v1/docker/containers");
    containers = ctn.containers || [];
  } catch { /* docker 不可用时跳过 */ }

  if (containers.length > 0) {
    lines.push("", "**Containers:**");
    for (const c of containers) {
      const isHealthy = (c.status as string).startsWith("Up");
      const icon = isHealthy ? "✅" : "⚠️";
      const portStr = (c.ports as string[] || []).join(", ") || "-";
      lines.push(`- ${icon} \`${c.name}\` ${c.status} | ports: ${portStr}`);
    }
  } else {
    lines.push("- No containers found");
  }

  // 端口不通时，主动提示异常容器
  if (!found) {
    const broken = containers.filter((c: any) =>
      !(c.status as string).startsWith("Up")
    );
    if (broken.length > 0) {
      lines.push("", "**⚠️ Potential cause — non-running containers:**");
      for (const c of broken) lines.push(`  - \`${c.name}\`: ${c.status}`);
    }
  }

  const target = args.domain || args.server_id;
  return {
    content: [{ type: "text" as const, text: `## Diagnose: ${target}:${port}\n\n${lines.join("\n")}` }],
  };
}

// M5: diagnoseWebsiteHandler — 增强诊断含 HTTP GET 探测

// M5: serverListHandler — 返回可用的 server 列表
export async function serverListHandler(_args: Record<string, unknown>) {
  const lines = ["## Servers", "", "| Server ID | Endpoint | Status |", "|-----------|----------|--------|"];

  const servers = listEndpoints();
  if (servers.length === 0) {
    lines.push("| - | no agents configured | - |");
  } else {
    const checks = await Promise.all(
      servers.map(async (s) => {
        const status = await new AgentClient(s.id).get("/health")
          .then(() => "🟢 online")
          .catch(() => "🔴 offline");
        return { ...s, status };
      })
    );
    for (const s of checks) {
      lines.push(`| ${s.id} | ${s.endpoint} | ${s.status} |`);
    }
    lines.push("", `**Total:** ${servers.length} server(s)`);
  }

  return {
    content: [{ type: "text" as const, text: lines.join("\n") }],
  };
}

// M3: applyDeployHandler — 部署是 high risk 操作，必须和 plan.apply 一样过安全闸门。
// Agent 侧 ValidateCompose 检出 supply chain 风险（privileged/docker.sock/root mount/host net）
// 返回 409。MCP 层接住 409 → 返回风险卡片，指示 AI 停下问用户，不自行 force。
export async function applyDeployHandler(args: {
  plan_id?: string;
  repo_url: string;
  branch?: string;
  app_name?: string;
  confirm?: boolean;
  server_id?: string;
}) {
  try {
    const d = await client(args.server_id).post("/api/v1/deploy/apply", {
      plan_id: args.plan_id || "plan",
      repo_url: args.repo_url,
      branch: args.branch || "main",
      app_name: args.app_name || "",
      force: args.confirm === true,
    });
    const hc = d.healthcheck || {};
    const text = `## Deploy: ${d.status}\n| Field | Value |\n|-------|-------|\n| App | ${d.app_name} |\n| Release | ${d.release_id || "-"} |\n| Runtime | ${d.runtime || "-"} |\n| Healthcheck | ${hc.status || "-"} (${hc.latency_ms || 0}ms, HTTP ${hc.status_code || "-"}) |\n${d.error ? `| Error | ${d.error} |` : ""}`;
    return { content: [{ type: "text" as const, text }], structuredContent: d };
  } catch (e) {
    if (e instanceof AgentError && e.status === 409) {
      let risks: string[] = [];
      try { risks = JSON.parse(e.body).risks || []; } catch { /* body 非 JSON，用空列表兜底 */ }
      const text = [
        `## ⚠️ 部署被安全闸门拦截`,
        ``,
        `检测到 supply chain 风险配置：`,
        ...(risks.length ? risks.map((r) => `- 🔴 ${r}`) : [`- 🔴 危险配置（详见 compose 文件）`]),
        ``,
        `这些配置可能让容器控制宿主机。**请将本卡片展示给用户，不要自行决定。**`,
        `仅当用户审阅风险并明确表示确认后，再调用 \`app.apply_deploy\` 并传入 \`confirm: true\`。`,
      ].join("\n");
      return { content: [{ type: "text" as const, text }] };
    }
    throw e;
  }
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}
