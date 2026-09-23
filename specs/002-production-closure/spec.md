# Feature Specification: Production Closure & Final Audit

**Feature Branch**: `main`
**Created**: 2026-09-24
**Status**: In Progress
**Input**: User request — "完整落地闭环所有任务，真实 E2E 测验、验收、审计、提交推送仓库、创建发行版；以生产级 SaaS 标准全面审查（高并发/性能/安全/慢 SQL/契约/UI/可观测性），按 Spec Kit 规范执行。"

## User Scenarios & Testing

### User Story 1 - Production deployability (Priority: P1)
用户/运维可按文档一键构建镜像并部署，服务健康检查通过，`/api/status` 返回正确版本。

**Independent Test**: `docker build` + compose up + curl /api/status + X-New-Api-Version 头。

### User Story 2 - Billing transparency (Priority: P1)
真实用户能在日志详情看到费用分项（模型/分组/补全/缓存倍率、命中档位、影子价），管理员能看到配额饱和审计标记。

**Independent Test**: 本地 mock 上游 + 真实请求 → 日志详情「费用明细」面板渲染；管理端日志详情渲染 quota_saturation。

### User Story 3 - Fault tolerance (Priority: P1)
坏渠道（冷却/低健康分）不被普通渠道选择器选中；流式首包失败自动换渠道（默认开启）。

**Independent Test**: `TestChannelHealthRoutingFilter` + stream_fallover 默认值测试。

### User Story 4 - Observability (Priority: P1)
任一请求可被 request-id 串起；时间线展示真实阶段耗时（非推测）。

**Independent Test**: `TestBuildTimelineStages` + 前端 request-timeline 测试。

### User Story 5 - Plugin sandbox & marketplace safety (Priority: P2)
JS 任务插件受沙箱限制（超时/内存/网络/fs），marketplace 拒绝非法 URL。

**Independent Test**: 恶意脚本（死循环/超大内存/网络）被拦截；非法 URL 拒绝。

### User Story 6 - OWASP ASVS security audit (Priority: P1)
认证/会话/访问控制/数据保护按 ASVS V2/V3/V4/V6 逐项核对，高危缺口已修。

**Independent Test**: `计划书/audit/security-asvs-audit.md` 审计报告 + 修复回归测试。

## Requirements

### Functional Requirements
- FR-001: 插件执行默认受沙箱限制（超时、内存、网络/fs 白名单）
- FR-002: marketplace 安装仅允许 https URL 且来源校验
- FR-003: 错误码（429/502/504/额度用尽/渠道无可用）映射为小白人话，7 语言
- FR-004: 关键交互（登录/渠道/日志/钱包/任务插件）有 loading/成功/失败/空态反馈
- FR-005: 三库矩阵（SQLite/MySQL/PG）迁移幂等检查脚本可用
- FR-006: 建立 `计划书/db_structure.md`（表/索引/约束权威源）与 `计划书/project_specs.md`（任务进度权威源）
- FR-007: 验证台账 `计划书/audit/perf-verification-ledger.md` 记录已验证范围，防止重复跑
- FR-008: 术语表补充计费/限速/任务/Web 防护术语，7 语言一致
- FR-009: 生成式 AI 合规 checklist（备案/算法备案/实名/日志/内容安全）

### Key Entities
- Channel / ChannelHealthSnapshot / ChannelFilter
- Log / LogOther (timeline_stages, quota_saturation, explain)
- TaskPlugin / jsplugin SecurityPolicy / ExecutionGate
- WebProtectionSetting / ServerStats
- User / UserSession / AuditLog

## Success Criteria
- SC-001: `go vet ./...`、`go build ./...`、`go test ./...`（相关包）exit 0
- SC-002: `bun run typecheck`、`bunx vitest run`（相关 feature）exit 0
- SC-003: 新功能用户可用（真实浏览器/上游/E2E 证据）入 `计划书/e2e-evidence/`
- SC-004: 主题 commit + tag/Release + 回滚路径记录
- SC-005: `graft build` 刷新 + 计划书进度/台账更新

## Assumptions
- 子代理通道当前不可靠（宿主 CC Switch 网关故障），主控单线程推进为主
- 本机无 docker CLI：三库矩阵/镜像构建在服务器或 CI 验证，如实标注
- 生产部署（线上 freeapi.tingfengai.art）需用户明确授权 SOP
- 旧 `计划书/` 删除状态为用户既有工作，T14 清理待授权

## Edge Cases
- 无样本（新渠道）健康分过滤必须 fail-open（不剔除新渠道）
- 旧日志无 timeline_stages/explain → 前端优雅降级
- 插件沙箱必须 fail-closed（未声明权限拒绝执行）
- 普通用户日志视图必须剥离 admin_info（含 quota_saturation）