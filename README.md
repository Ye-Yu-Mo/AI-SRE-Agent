# AI Server Agent

让 AI Agent 安全地部署、管理和维护真实 Linux 服务器。

**AI 不直接操作 shell。** AI 通过 typed action 理解服务器状态、生成结构化计划、经风险评估后在受控沙箱中执行，所有操作可审计、可回滚。

## 架构

```
AI Client (Claude Code)
    │  MCP (stdio JSON-RPC)
    ▼
MCP Server (Node.js)          ← 13 个 MCP tools
    │  HTTP + shared secret
    ▼
Server Agent (Go)             ← systemd service, 运行在 Ubuntu 上
    │  D-Bus / Docker socket / /proc
    ▼
真实 Linux 服务器
```

## 快速开始

### 1. 在目标服务器安装 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/ai-sre/agent/main/install.sh | sh
```

Agent 将以 systemd service 运行在 `9090` 端口。安装完成后记录生成的 `AGENT_SECRET`。

### 2. 本地启动 MCP Server

```bash
git clone https://github.com/ai-sre/agent.git
cd agent/mcp-server
npm install && npm run build
```

### 3. 配置 Claude Code

在项目根目录创建 `.mcp.json`：

```json
{
  "mcpServers": {
    "ai-server-agent": {
      "command": "/path/to/mcp-server/run.sh",
      "args": [],
      "env": {
        "AGENT_ENDPOINT": "http://<你的服务器IP>:9090",
        "AGENT_SECRET": "<安装时生成的secret>"
      }
    }
  }
}
```

重启 Claude Code，即可在会话中使用 MCP tools。

## MCP Tools

### Read — 查看状态
| Tool | 功能 |
|------|------|
| `server.inspect` | CPU/Mem/Disk/OS/Kernel/Arch/Ports |
| `server.health` | 健康检查 + 告警 |
| `server.resources` | 详细资源数值 |
| `service.list` | systemd 服务列表 |
| `service.logs` | journal 日志 |

### Write — 执行写操作
| Tool | 功能 |
|------|------|
| `service.plan_restart` | 生成重启计划（不直接执行） |
| `plan.apply` | 执行计划 |

### Audit — 审计
| Tool | 功能 |
|------|------|
| `audit.search` | 查询操作审计日志 |

### Deploy — 部署
| Tool | 功能 |
|------|------|
| `app.plan_deploy` | 生成部署计划 |
| `app.apply_deploy` | 执行部署（clone → build → up → healthcheck → release） |
| `app.status` | 查看应用状态和 release 信息 |
| `app.rollback` | 回滚到上一版本 |

### Diagnosis — 诊断
| Tool | 功能 |
|------|------|
| `diagnose.website` | 诊断网站不可访问原因 |

## 安全原则

| 原则 | 实现 |
|------|------|
| 不暴露 root shell | AI 只能调用 typed action，不能执行任意命令 |
| Plan/Apply 分离 | 有副作用的操作先生成计划，审批后执行 |
| 默认最小权限 | 每个 session 只获得完成任务所需的最小 capability |
| 所有操作可审计 | 每次写操作记录 before/after state、stdout/stderr |
| 部署可回滚 | 每次部署创建 release record，失败自动回滚 |
| 命令沙箱 | 危险命令（rm -rf /、mkfs、passwd 等）默认拒绝 |

## 项目结构

```
├── agent/                  # Go Agent — 运行在目标服务器
│   ├── cmd/agent/          # CLI 入口 (ai-server-agent serve)
│   ├── internal/
│   │   ├── action/         # Typed Action 模型 + Plan 状态机
│   │   ├── collector/      # /proc, systemd, Docker 状态采集
│   │   ├── deploy/         # 部署流水线 (clone, compose, healthcheck, release, rollback)
│   │   ├── executor/       # Typed Executor + 命令沙箱
│   │   ├── identity/       # Server identity 生成
│   │   ├── plan/           # Plan 内存存储
│   │   ├── risk/           # 硬编码风险分级表
│   │   ├── service/        # Systemd unit 模板
│   │   └── storage/        # 本地 JSON 文件存储 (audit + snapshots)
│   ├── install.sh          # 一行安装脚本
│   └── uninstall.sh        # 卸载脚本
├── mcp-server/             # MCP Server — AI 交互层
│   └── src/
│       ├── index.ts        # 13 个 MCP tool 注册
│       ├── client/agent.ts # Agent HTTP client
│       └── tools/server.ts # Tool handlers
├── .mcp.json               # Claude Code MCP 配置示例
├── PLAN.md                 # 里程碑计划
├── MCP-DEV.md              # MCP 开发指南
├── test_project/           # 集成测试
└── README.md
```

## 技术栈

| 组件 | 技术 |
|------|------|
| Agent | Go, 单一静态二进制 (~8MB) |
| MCP Server | TypeScript, @modelcontextprotocol/sdk |
| Agent 存储 | JSON 文件 (Phase 0) |
| 部署 | systemd service + docker-compose |
| 传输 | Agent ↔ MCP Server: HTTP + shared secret |
