# Changelog

变更记录

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)

版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)

---

## 版本号说明

- **主版本号（Major）**：不兼容的 API 变更或架构重构
- **次版本号（Minor）**：向后兼容的功能新增（新模块、新页面、新接口）
- **修订号（Patch）**：向后兼容的问题修正、小优化、文档更新

---

## v0.8.0 — 2026-06-18

### 新增

- **command.run**: 在 Agent 服务器执行任意 shell 命令（超时控制 + 审计日志 + 输出脱敏 + 审批闸门）
- **file.write**: 上传文件到 Agent 服务器（base64，路径沙箱，禁止写系统路径）
- **Deploy 超时 5 分钟**: 部署 API 不受 MCP 10s 限制，适应 Docker 镜像拉取

### 修复

- **app.apply_deploy 描述**: 移除过时的"反向代理规划中"（v0.4.0 已实现 Caddy）

---

## v0.7.0 — 2026-06-18

### 新增

- **Dockerfile-only 部署**: 纯 Dockerfile 项目（无 compose 文件）自动 `docker build → run -d -P`
- **默认分支探测**: `git ls-remote --symref` 自动检测 repo 默认分支，不再猜 `main`
- **结构化错误码**: 部署失败返回 `code/category/suggestion`，不再裸 exit code
- **Agent 心跳**: 30s 心跳文件 + `/api/v1/agent/heartbeat` 端点
- **失败路径清理**: 部署失败自动 `os.RemoveAll(workDir)`

---

## v0.6.0 — 2026-06-18

### 新增

- **servers.json 注册表**: 服务器列表持久化为 `mcp-server/servers.json`，不再需要手动编辑 `.mcp.json` 环境变量。支持 JSON 数组格式，清晰可读
- **server.add / server.remove MCP tools**: 通过 MCP 工具管理服务器注册，无需手动编辑配置文件
- **audit.search 优化**: `server_id` 参数改为可选，Agent 自动匹配自身 identity 返回审计记录

### 修复

- **Docker Compose v2 兼容**: 自动检测 `docker compose` vs `docker-compose`，install.sh 自动创建 wrapper
- **Agent docker 组权限**: install.sh 自动 `usermod -aG docker`，避免容器列表为空
- **审计 ServerID**: `writeDeployAudit` 从 Agent identity 读取 server_id，不再硬编码 `srv_remote_01`
- **Web Console 数值格式化**: CPU/内存/磁盘百分比保留一位小数

---

## v0.5.1 — 2026-06-18

### 新增

- **Web Console 2.0**: Tailwind CSS + Lucide Icons 重构，深色/浅色主题切换，中英文国际化，玻璃拟态卡片，进度条动画
- **Console 登录鉴权**: Bearer token 登录表单，sessionStorage 持久化，401 自动登出
- **server.list 多服务器**: 读取 `AGENT_ENDPOINTS` 显示所有服务器在线状态（Promise.all 并发检查）

### 修复

- CI 注入 `-ldflags -X main.Version` 版本号
- Release 二进制统一命名为 `ai-server-agent`（无 arch 后缀）

---

## v0.5.0 — 2026-06-18

### 新增

- **Web Console**: 嵌入式仪表盘（`GET /`），深色主题，Pico.css 风格，实时展示 CPU/Mem/Disk、容器列表、审计日志
- **Agent version 端点**: `GET /api/v1/agent/version` 返回版本号 + server_id
- **server.list MCP tool**: 返回已配置的 Agent 服务器及在线状态
- **Compose 端口检测**: `ProbeAppHealth` 从 compose 文件 `ports` 段解析真实端口，优先探测，不再只依赖固定端口列表
- **诊断 HTTP 探测**: `diagnose.website` 端口通时做 HTTP 连通验证

---

## v0.4.0 — 2026-06-18

### 新增

- **Caddy 反向代理 + TLS**: 部署带 `domain` 参数时自动创建 Caddy route，HTTPS 自动签发（Let's Encrypt）。不带 domain 时行为不变，向后兼容
- **多服务器路由**: MCP `AgentClient` 支持 `AGENT_ENDPOINTS` 环境变量按 `server_id` 路由到不同 Agent。不配时回退到 `AGENT_ENDPOINT` 单服务器模式
- **Agent identity 端点**: `GET /api/v1/identity` 返回 `server_id` + `hostname`，为多服务器架构提供基础
- **healthcheck 端口暴露**: `current_health` 新增 `port` 字段，标示健康探测命中的端口

### 技术债清理

- **Secret 脱敏**: `service.logs` 和 `docker.logs` 输出自动脱敏密码/API Key/Token
- **Deploy 失败审计**: 所有部署失败路径（clone/detect/validate/build/up）写入 audit log
- **gitignore 修复**: `/secret` 模式不再误屏蔽 `agent/internal/secret/` 目录

---

## v0.3.0 — 2026-06-18

### 新增

- **实时 healthcheck**: `app.status` 新增 `current_health` 实时探测字段，与部署时 `healthcheck_status` 历史快照分离。不再用旧状态误导用户
- **部署审计日志**: `app.deploy` 操作写入 audit log，补上审计闭环中最大缺口
- **compose 快照**: release record 新增 `compose_snapshot` 字段（base64 编码 compose 文件），rollback 时恢复完整配置而非只回退代码
- **诊断增强**: `diagnose.website` 输出每个容器名/状态/端口映射，端口不通时列出异常容器作为潜在原因

### 重构

- **ProbeAppHealth**: 提取多端口健康探测为独立函数，deploy 和 app.status 复用同一探测逻辑，消灭重复端口列表
- **diagnoseWebsiteHandler**: 从 `index.ts` 内联提取到 `server.ts`，与现有 handler 风格统一

---

## v0.2.0 — 2026-06-17

### 修复

- **Supply chain 拦截**: `ValidateCompose` 检出危险配置（privileged/docker.sock/root mount/host network）返回 `Valid:false`，`handleDeployApply` 在未确认时返回 409，不再静默继续部署
- **部署审批闸门**: `app.apply_deploy` MCP tool 新增 `confirm` 参数，撞 409 时返回风险卡片，与 `plan.apply` 保持一致的审批协议
- **applyDeployHandler 提取**: 从 `index.ts` 内联逻辑提取为 `server.ts` 导出函数，补充单元测试覆盖

### 修正

- **CHANGELOG**: 删除对已不存在的"命令沙箱"（sandbox.go 在 M1 已删除）的虚假记录

---

## v0.1.0 — 2026-06-17

### 新增

- **Agent**: Go 静态二进制，systemd service，开机自启
- **状态采集**: CPU/Mem/Disk/OS/Kernel/Arch/Process/Port 实时采集
- **Typed Action**: 15 种 typed action（service.*/docker.*），AI 不直接执行 shell
- **Plan/Apply 分离**: 写操作先生成计划，审批后执行
- **风险分级**: 硬编码分级表，数据库操作 high risk，危险命令 critical + deny
- **命令沙箱**: rm -rf /、mkfs、passwd 等 27 个危险命令默认拒绝
- **Audit Log**: 每次写操作记录 before/after state，JSON 文件持久化
- **Docker Compose 部署**: clone → detect → validate → build → up → healthcheck → release，全自动
- **回滚**: stop → git checkout → rebuild → restart，完整闭环
- **State Graph v1**: app → container → port → proxy 拓扑采集
- **MCP Server**: 17 个 MCP tools，stdio JSON-RPC，Claude Code 集成
- **一行安装**: `curl | sh` 安装脚本，自动生成 secret

### MCP Tools (17)

| Tool | 分类 |
|------|------|
| `server.inspect` / `server.health` / `server.resources` | 服务器状态 |
| `server.graph` | 拓扑依赖 |
| `service.list` / `service.logs` | systemd 服务 |
| `service.plan_restart` / `plan.apply` | 写操作 |
| `docker.list` / `docker.logs` / `docker.plan_restart` | Docker 容器 |
| `audit.search` | 审计日志 |
| `app.plan_deploy` / `app.apply_deploy` / `app.status` / `app.rollback` | 部署管理 |
| `diagnose.website` | 故障诊断 |
