# v1.3.60 本地真实浏览器 E2E 证据

**方式**：Playwright（Chromium）真实浏览器操作本地网关
`PORT=3000 SQLITE_PATH=e2e-v1360.db go run main.go`（v1.3.60 代码）
**结果**：13/13 通过（`results.json`）

| # | 断言 | 证据 |
|---|---|---|
| 1 | 登录成功 | 01-login.png / 02-dashboard.png |
| 2 | 数据库备份分区可见（侧栏「数据库备份」入口） | 03-db-backup-section.png |
| 3 | 导出/导入按钮存在 + 防错提示（不删除不覆盖） | 03-db-backup-section.png |
| 4 | **真实导出并下载**（gzip 有效） | backup-export.sqljson.gz（1964 bytes，gzip magic 1f 8b） |
| 5 | 密钥列表非空（防假阳性前提） | 04-keys-list.png（rows=2） |
| 6 | 找到掩码密钥触发器 | 06-after-reveal-key.png |
| 7 | **查看自己的密钥未弹二次验证** | 06-after-reveal-key.png（无验证弹窗） |
| 8 | **完整密钥确实被揭示**（正向证据） | 06-after-reveal-key.png 顶部 toast「API Key 已解锁」 |
| 9 | 浏览器控制台无致命错误 | results.json |

## 反向安全验证（API 层，防「放宽=越权」）
```
# 读自己的（无 X-Security-Proof）→ 200 + 明文
POST /api/token/2/key                  → {"key":"aUrQONmX..."}  200
# 批量读自己的 → 200
POST /api/token/batch/keys {"ids":[2]} → {"keys":{"2":"aUrQONmX..."}}  200
# 越权读他人 → 404 + TOKEN_NOT_FOUND（不放行、不泄露）
POST /api/token/2/key (as other user)  → 404 TOKEN_NOT_FOUND
# 越权批量 → 404（非 200 空 map）
POST /api/token/batch/keys (as other)  → 404 TOKEN_NOT_FOUND
```
单测锁定同上契约：`controller/TestTokenKeyDisclosureOwnershipAndStatus`。

## 未部署说明
按用户指示处于**部署冻结期**：本批未执行线上部署，故以本地真实浏览器 E2E 作为完成度凭证。
