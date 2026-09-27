# 实施计划：站点订阅运营统计（T15-A）

**分支**: `main` | **日期**: 2026-09-27 | **Spec**: `specs/001-subscription-site-stats/spec.md`

## 摘要
公开只读站点订阅聚合统计，聚合既有 `user_subscriptions` / `subscription_plans`，零新增表、零网络外呼。

## 技术上下文
- **语言**: Go 1.25 + React 19 / TS 5.9
- **存储**: 既有主库三表（SQLite/MySQL/PostgreSQL 兼容，全 GORM）
- **测试**: Go `testify`（controller 单测）+ vitest（前端组件）
- **性能**: COUNT 聚合，P0 规模（<10 万订阅）<200ms

## 实施
1. **后端**（已完成）
   - `model/subscription_stats.go`：聚合查询
   - `controller/site_stats.go`：HTTP 处理器
   - `router/relay-router.go`：`GET /v1/stats/subscriptions`（限流、公开）
2. **前端**（已完成）
   - `web/src/features/pricing/components/site-subscription-stats-card.tsx`：定价页只读卡
   - i18n 7 语言（en/zh/zh-TW/fr/ru/ja/vi）
3. **测试**（已完成）
   - `controller/site_stats_test.go`：聚合口径 + 无明细泄露
   - 前端 typecheck / vitest

## 验收（v1.3.46）
- `GET /v1/stats/subscriptions` 本地网关实测返回正确聚合（计划书/e2e-evidence/）。
- 定价页真实浏览器冒烟展示统计卡。