# T15 立项任务卡（Task Cards）

> 生成：2026-09-24 · 由 t15-decision-record.md 决策驱动的落地任务卡。
> 原则：每张卡「方案 → 任务 → 验收」，不伪造未验证的上线。

## 任务卡 T15-1 · 模型目录同步 dry-run（✅ 已落地）

**目标**：只读拉取上游模型目录 + diff 报告，人工审批后才写入，防价格漂移。

**落地证据**（代码实锤）：
- `GET /api/models/sync_upstream/preview`（dry-run diff 预览）
  - router/api-router.go:407
  - controller.SyncUpstreamPreview（controller/model_management_test.go:620 测试绿）
- `POST /api/models/sync_upstream`（审批后写入）
  - router/api-router.go:408；controller.SyncUpstreamModels（测试 646-687 覆盖空/错误/成功路径）
- 前端 Sync Wizard 四步流程（Select → Preview fields → Confirm → Sync results）：
  - web/src/features/models/components/dialogs/sync-wizard-dialog.tsx
  - 按钮入口 models-primary-buttons.tsx 「Sync metadata」/「Sync pricing」

**验收**：`go test ./controller/ -run TestModelManagementDatabaseMatrix` → sqlite 全绿
（mysql/pg SKIP 因本机无 docker，服务器 compose 已有三库矩阵记录）。

## 任务卡 T15-2 · 任务品类扩展（📋 已立项，待上游凭证）

**目标**：按决策「补 2-3 个高需求任务模板」，扩大网关覆盖面。

**现状**（已有基础）：
- 10 个任务插件：kling/sora/jimeng/vidu/hailuo/doubao/google/alibaba/sunoapi/vertex-ai
  （plugins/tasks/*/plugin.js，manifest + JS 逻辑 + 沙箱）
- jsplugin 沙箱（T6 加固）+ plugin_protocol 桥接 + taskcommon 模板范式

**立项范围**（下批执行，每项需真实上游凭证才能 E2E）：
1. 视频生成补充模板（如已覆盖渠道的 v2 接口对齐）
2. 文档解析类模板（document-parser 类，文本上游更易验证）
3. 语音/TTS 类模板（sunoapi 已有先例）

**验收**（每模板）：manifest 注册 + 本地 mock 上游 E2E + 计费表达式复用 T4 + 沙箱测试绿。
**阻塞**：无真实上游 API key 前不宣称上线，仅交付模板范式文档。

## 任务卡 T15-3 · Electron 桌面增强（可选，未立项）

按 decision record 暂缓：需先确认是否有真实桌面用户场景。
