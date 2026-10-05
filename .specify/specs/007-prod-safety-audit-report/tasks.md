# 007 — Tasks（任务清单）

> 状态：☐ 待办 ｜ ◐ 部分 ｜ ☑ 完成（有证据）｜ ⊘ 受阻/披露

## US-1 暴力破解防护（P0）
- ☑ `login_rate_limit_setting.go` Enabled 默认 true
- ☑ `RecordLoginLog` 增 success/status；新增 `RecordLoginFailureLog`
- ☑ `controller/user.go` 失败分支调 `recordLoginFailureAudit`（去敏）
- ☑ `controller/login_audit_test.go` 3 用例 + 先红后绿
- ☐ 注册/OAuth 侧邮箱枚举（未做，spec §4 披露）

## US-2 私网判定一致（P0）
- ☑ `common/ip.go` 补 CGNAT/link-local/保留段
- ☑ `common/ip_test.go` + 先红后绿

## US-3 用量报表（P1）
- ☑ `model/usage_report.go`（整数日键 / `dim_key` 别名）
- ☑ `controller/usage_report.go`（JSON + CSV 流式）
- ☑ `router/api-router.go` 路由
- ☑ 三库 conformance `TestDBConformanceUsageReport`（连跑 2 次）
- ☑ 前端 dashboard 第 5 section + i18n 14 键 × 7 语言

## US-4 CORS（P1）
- ☑ `middleware/cors.go` env 白名单 + 消除冲突
- ☑ `middleware/cors_test.go` 5 用例 + 先红后绿

## US-5 稳定性（P2）
- ☑ 批量额度优雅关闭（`model/utils.go` + `main.go`）
- ☑ 后台 loop 优雅关闭（`service/background_loop.go` + 4 loop）+ 真机 SIGTERM 验收
- ☑ 删除 `idx_logs_traffic` + 三库验证

## 文档同步
- ☑ README.md（env 表 4 项 + 特性节）
- ☑ `计划书/project_specs.md`（T12 证据路径修正）
- ☑ `计划书/docs/t8-security-asvs-deepdive.md`（G4 更正为部分闭环）
- ☑ `.specify/specs/007-...`（spec/plan/tasks）
- ☑ `计划书/workflow_status-4x-batch.md`

## 后续批次（未完成，诚实披露）
- ⊘ 视频/图像端点适配（OpenAI 契约）
- ⊘ 图床 + 5 分钟清理参考图素材
- ⊘ 4.6.1 媒体 Provider 适配层 / 4.6.2 产物质检闸门
- ⊘ 4.7.1 用户记忆层 / 4.8.1 任务感知路由
- ⊘ 4.9.3 剩余 10 处 alert-dialog 复用
- ⊘ HTML 报告 + 测验（见 计划书/reports/）
