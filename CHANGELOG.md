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
