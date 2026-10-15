# SOP — 回滚：安全地退掉一批新功能

> 面向「新上线的功能要退回去」的场景。核心原则：**能被开关关掉的，就不要动代码；
> 动代码前先确认新增的持久化状态与外部副作用如何处理。**
> 最后核对：2026-10-16（对照 `VERSION` = `v1.3.147`）。

---

## 0. 回滚前必答的三个问题

1. **这批改动有没有外部副作用？**（数据已上传云盘、已写审计、已发通知）——有则无法用代码回滚抵消，
   需单独清理。
2. **有没有开关能关掉它？**——优先用开关（零代码回滚、可逆）。
3. **新增的持久化状态（表 / options 键）会不会在回滚后造成问题？**——GORM `AutoMigrate` 只加不删，
   旧代码不会因为多了一张表而崩。

---

## 1. 首选：用开关关功能（无损、可逆）

### 1.1 关掉「训练数据采集」

- 管理端：`PUT /api/admin/training-data/enabled`，body `{"enabled": false}`
  （路由 `router/training-data-router.go:37`）。
- 或直接改 `options` 表键 `TrainingDataCaptureGatewayEnabled = false`
  （`service/training_capture/gateway_option.go:19-23`）。
- 效果：`IsGranted` 第一道闸门即返回 false（`service/training_capture/consent.go:170`），
  热路径快速开关同步为 false（`consent.go:120`），**立即停止新采集**。
- 用户侧：撤回某用户授权（`TrainingDataConsent`）立即失效其缓存并停止采集（`consent.go:14-37`）。

### 1.2 调回资源上限（若担心内存/带宽）

- `TRAINING_CAPTURE_MAX_TEXT_MB` / `TRAINING_CAPTURE_MAX_PART_MB` / `TRAINING_CAPTURE_QUEUE_CAP` /
  `TRAINING_CAPTURE_BUDGET_PCT` / `TRAINING_CAPTURE_MIN_BUDGET_MB`
  （`service/training_capture/resources.go:70-76`）。

---

## 2. 数据库：新增的表怎么处理

本批新增 4 张表（`model/main.go:384-385,436` 登记 AutoMigrate）：

| 表 | 模型 | 说明 |
|---|---|---|
| `training_data_consents` | `TrainingDataConsent`（`model/training_data.go:55-57`） | 用户授权状态 |
| `training_data_watermarks` | `TrainingDataWatermark`（`model/training_data.go:107-109`） | 会话采集水位 |
| `training_data_exports` | `TrainingDataExport`（`model/training_data_export.go:80-82`） | 导出记录 |
| `training_data_export_downloads` | `TrainingDataExportDownload`（`model/training_data_export.go:128-130`） | 导出下载记录 |

- **回滚代码不会自动删表**（AutoMigrate 只加不删）。表留着**无害**——旧代码不读它们。
- 若确需彻底删除：手工 `DROP TABLE`（**不可逆、需明确授权**，且会丢授权与水位状态）。
- 三库差异注意：SQLite 删列/改列能力弱，删列用 `DROP TABLE` 或重建；不要用 `ALTER COLUMN`
  （`AGENTS.md:108`）。

---

## 3. 路由

- 本批新增路由：`/api/admin/training-data/*`、`/api/admin/ops/*`
  （`router/training-data-router.go:35-50`）；用户端 `/api/user/...training-data`（`api-router.go` 内）。
- 回滚到旧镜像即恢复旧路由表。**旧前端不会调用新路径**，所以不会出现 404（404 只发生在
  「新前端 + 旧后端」这种半回滚状态）。
- 若只回滚前端不回滚后端：多余的端点只是不可达，无副作用（但管理端页面会消失）。

---

## 4. 前端

- 回退文件：`web/src/features/training-data/`、`web/src/features/ops-panel/`、`web/src/features/profile/`
  相关改动，以及路由/侧边栏入口。
- **前端改动 = 重建镜像**：`web/dist` 经 `go:embed`，改前端必须 `bun run build` 并 `docker build` 重编，
  不是热更新（`.claude/skills/new-api-add-feature/SKILL.md` §12）。
- i18n 回滚时**不要**用整文件 re-serialize 工具碰 `web/src/i18n/locales/*.json`——文件里有混淆键
  `footer.newapi.projectAttributionSuffix`（`web/src/i18n/locales/en.json:2590`），普通 JSON 工具会把它
  解码成明文，破坏受保护标识（`AGENTS.md:166-173`；`web/scripts/sync-i18n.mjs:25-30,122-128` 有保形逻辑）。

---

## 5. 镜像 / 部署回滚

- 零停机栈：`rollback.sh <上一 tag>`，或用 `deploy-zero-downtime.sh <上一 tag>` 指定旧镜像
  （见 `.claude/skills/new-api-add-feature/SKILL.md` §7；服务器脚本 `/opt/new-api/rollback.sh`）。
- 注意部署脚本 `docker run` 必须含 `--memory 900m --memory-swap 900m`，否则回滚脚本本身可能触发
  全局 OOM（2026-10-07 事故记录）。

---

## 6. ★ 不可回滚的东西（回滚前必须知道）

- **已上传到云盘（OneDrive）的采集数据是外部的**：代码/开关回滚不会删除已外发数据。
  清理需在管理端 `/api/admin/training-data/file`（`router/training-data-router.go:42`）或云盘侧进行。
  这是**不可逆外发**——回滚决策时必须显式考虑隐私影响。
- **已写的审计记录**（如 `user.oauth.legacy_migration_refused`，`controller/oauth.go:462-467`）不可撤回。

---

## 7. ★ 不应回滚的安全修复

本批的安全修复**方向是收敛而非放宽**，回滚它们会重新引入漏洞：

- OAuth 历史绑定迁移要求「已验证邮箱一致」的 fail-closed 判据（`controller/oauth.go:443-477,654-685`）
  —— 回滚 = 重新打开**账号接管**入口。
- 授权服务端强制（`service/training_capture/consent.go`）—— 回滚 = 未授权数据可被采集。
- queue 不关数据通道（`service/training_capture/queue.go:61-88`）—— 回滚 = `send on closed channel` 杀进程。

→ 回滚只退**功能**，不退**安全收敛**。若必须退，先做影响分析并说明重新引入的风险。

---

## 8. 回滚后验证清单

- [ ] `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/readyz` → `200`
- [ ] `curl -sI https://<域名>/api/status | grep -i x-new-api-version` → 版本回到目标 tag
- [ ] `GET /api/admin/training-data/overview` → `enabled=false`
- [ ] `docker logs --since 5m new-api 2>&1 | grep -i "training capture"` → 不再产生新的采集/上传日志
- [ ] 云盘侧确认无新文件产生（管理端 files 列表）
