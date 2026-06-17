# AI Server Agent

让 AI Agent 安全地部署、管理和维护真实 Linux 服务器。

**AI 不直接操作 shell。** AI 通过 typed action 理解服务器状态、生成结构化计划、经风险评估后在受控沙箱中执行，所有操作可审计、可回滚。

## 架构

```mermaid
flowchart LR
    A["AI Client<br/>(Claude Code)"] -->|"MCP stdio JSON-RPC"| B["MCP Server<br/>(Node.js)<br/>17 个 MCP tools"]
    B -->|"HTTP + shared secret"| C["Server Agent<br/>(Go binary)<br/>systemd service"]
    C -->|"D-Bus / Docker socket / /proc"| D["Linux 服务器<br/>(Ubuntu 22.04+)"]
```

## 快速开始

### 1. 在目标服务器安装 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/Ye-Yu-Mo/AI-SRE-Agent/main/agent/install.sh | sh
```

安装完成后终端会打印 `AGENT_SECRET`，复制备用。Agent 以 systemd service 运行在 `9090` 端口。

### 2. 本地安装 MCP Server

```bash
git clone https://github.com/Ye-Yu-Mo/AI-SRE-Agent.git
cd AI-SRE-Agent/mcp-server
npm install && npm run build
```

### 3. 配置 Claude Code

在项目根目录创建 `.mcp.json`：

```json
{
  "mcpServers": {
    "ai-server-agent": {
      "command": "/path/to/AI-SRE-Agent/mcp-server/run.sh",
      "args": [],
      "env": {
        "AGENT_ENDPOINT": "http://<服务器IP>:9090",
        "AGENT_SECRET": "<安装时打印的secret>"
      }
    }
  }
}
```

重启 Claude Code 即可使用。

## MCP Tools（17 个）

### 服务器状态（只读）
| Tool | 功能 |
|------|------|
| `server.inspect` | CPU/Mem/Disk/OS/Kernel/Arch/Ports |
| `server.health` | 健康检查 + 告警列表 |
| `server.resources` | 详细资源数值（百分比） |
| `server.graph` | 应用/容器/端口/反向代理拓扑依赖图 |

### systemd 服务
| Tool | 功能 |
|------|------|
| `service.list` | 列出所有 systemd 服务及状态 |
| `service.logs` | journal 日志（最近 N 行） |
| `service.plan_restart` | 生成重启计划（不直接执行） |

### Docker 容器
| Tool | 功能 |
|------|------|
| `docker.list` | 列出所有容器及状态 |
| `docker.logs` | 容器日志（最近 N 行） |
| `docker.plan_restart` | 生成容器重启计划 |

### 执行 & 审计
| Tool | 功能 |
|------|------|
| `plan.apply` | 执行已审批的计划 |
| `audit.search` | 查询操作审计日志 |

### 部署管理
| Tool | 功能 |
|------|------|
| `app.plan_deploy` | 生成部署计划（检测运行时、评估风险） |
| `app.apply_deploy` | 执行部署：clone → build → up → healthcheck → release |
| `app.status` | 查看应用状态和当前 release 信息 |
| `app.rollback` | 回滚到上一版本 |

### 故障诊断
| Tool | 功能 |
|------|------|
| `diagnose.website` | 诊断网站不可访问（端口/容器/代理） |

## 安全原则

| 原则 | 实现 |
|------|------|
| 不暴露 root shell | AI 只能调用 typed action，不能执行任意命令 |
| Plan/Apply 分离 | 有副作用的操作先生成计划，审批后执行 |
| 风险分级 | 硬编码分级表：low / medium / high / critical |
| 命令沙箱 | 27 个危险命令（`rm -rf /`、`mkfs`、`passwd` 等）默认拒绝 |
| 全量审计 | 每次写操作记录 before/after state、stdout/stderr |
| 部署可回滚 | 每次部署创建 release record，失败可一键回滚 |

## 项目结构

```
├── agent/                  # Go Agent — 运行在目标服务器
│   ├── cmd/agent/          # 入口：ai-server-agent serve
│   ├── internal/
│   │   ├── action/         # Typed Action 模型 + Plan 状态机
│   │   ├── collector/      # /proc、systemd、Docker 状态采集
│   │   ├── deploy/         # 部署流水线（clone/compose/healthcheck/release/rollback）
│   │   ├── executor/       # Typed Executor + 命令沙箱
│   │   ├── graph/          # State Graph 拓扑采集
│   │   ├── identity/       # Server identity 生成
│   │   ├── plan/           # Plan 内存存储
│   │   ├── risk/           # 硬编码风险分级表
│   │   └── storage/        # JSON 文件持久化（audit + releases）
│   ├── install.sh          # 一行安装脚本
│   └── uninstall.sh        # 卸载脚本
├── mcp-server/             # MCP Server — AI 交互层
│   └── src/
│       ├── index.ts        # 17 个 MCP tool 注册
│       ├── client/agent.ts # Agent HTTP client
│       └── tools/server.ts # Tool handlers
├── .mcp.json               # Claude Code MCP 配置示例
├── PLAN.md                 # 里程碑计划
└── CHANGELOG.md
```

## 技术栈

| 组件 | 技术 |
|------|------|
| Agent | Go，单一静态二进制（~8MB），无外部运行时依赖 |
| MCP Server | TypeScript，@modelcontextprotocol/sdk |
| 持久化 | JSON 文件（audit.jsonl + releases.jsonl） |
| 部署 | systemd service + Docker Compose v2 |
| 传输 | HTTP + shared secret |
