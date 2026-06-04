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

const server = new McpServer({
  name: "ai-server-agent",
  version: "1.0.0",
});

// ── Read tools ──

server.registerTool(
  "server.inspect",
  {
    description:
      "查看服务器基本信息和资源使用情况。返回 CPU/Mem/Disk/OS/Kernel/Arch/Hostname。" +
      "用于了解服务器整体状态。调用时机：用户问'服务器状态怎么样'、'看看那台机器'。",
    inputSchema: {
      server_id: z.string().describe("目标服务器 ID，例如 'srv_prod_01'"),
    },
  },
  inspectHandler
);

server.registerTool(
  "server.health",
  {
    description:
      "快速健康检查，返回服务器健康状态和告警列表。" +
      "用于快速判断服务器整体是否正常。调用时机：用户问'服务器健康吗'、'有没有问题'。",
    inputSchema: {
      server_id: z.string().describe("目标服务器 ID"),
    },
  },
  healthHandler
);

server.registerTool(
  "server.resources",
  {
    description:
      "查看服务器详细资源使用（CPU/内存/磁盘百分比）。返回纯数值，适合做对比分析。",
    inputSchema: {
      server_id: z.string().describe("目标服务器 ID"),
    },
  },
  resourcesHandler
);

server.registerTool(
  "service.list",
  {
    description:
      "列出所有 systemd 服务及其运行状态（active/inactive/failed）。返回服务名称和状态。" +
      "用于了解服务器上跑了哪些服务、有没有挂掉的服务。",
    inputSchema: {
      server_id: z.string().describe("目标服务器 ID"),
    },
  },
  async (args) => {
    const data = await new AgentClient().get("/api/v1/services");
    const services = data.services || [];
    const text = [
      `## Services (${services.length})`,
      ``,
      `| Service | Status |`,
      `|---------|--------|`,
      ...services.map((s: any) => `| ${s.name} | ${s.status} |`),
    ].join("\n");
    return { content: [{ type: "text" as const, text }], structuredContent: data };
  }
);

// ── Write tools ──

server.registerTool(
  "service.plan_restart",
  {
    description:
      "生成重启 systemd 服务的计划。不会直接执行，需要用户审批后调用 plan.apply。" +
      "返回风险等级、影响分析和 plan_id。" +
      "调用时机：用户说'重启 nginx'、'restart 那个服务'。",
    inputSchema: {
      server_id: z.string().describe("目标服务器 ID"),
      service_name: z
        .string()
        .describe("systemd 服务名称，如 'nginx'、'docker'"),
    },
  },
  planRestartHandler
);

// ── Start ──

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch((err) => {
  console.error("MCP Server fatal:", err);
  process.exit(1);
});
