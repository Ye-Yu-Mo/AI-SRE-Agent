import { AgentClient } from "../client/agent.js";

function client(): AgentClient {
  return new AgentClient();
}

export async function inspectHandler(args: { server_id: string }) {
  const data = await client().get("/api/v1/inspect");

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
  const data = await client().get("/api/v1/health");

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
  const data = await client().get("/api/v1/resources");

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
  const data = await client().post("/api/v1/plans", {
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

function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}
