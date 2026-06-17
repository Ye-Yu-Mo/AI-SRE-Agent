# Changelog

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
