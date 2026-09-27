# 终局闭环总审计 — 需求追踪矩阵与自我反向审判（v1.3.46）

> 日期：2026-09-27 · 用途：对照完整上下文建立「需求 → 实现 → 状态 → 证据 → 缺口」映射，
> 并对自己此前工作做最狠反向审判；作为 HTML 变更报告与验收测验的事实基础。
> 证据标记：`已验证`（真实运行）/ `静态确认`（读代码）/ `合理推断` / `待验证`。

## 一、显式需求追踪

| # | 需求（来源：本会话上下文） | 实现位置 | 状态 | 证据 | 缺口/动作 |
|---|---|---|---|---|---|
| R1 | 计划书/docs/ 全部文档主题落地闭环（T1–T15） | docs/t1…t15 + 代码 | ✅ 已闭环（T8-G1/G4/G5、T15-A/B 本批补完；C 复核既有） | 各文档闭环状态段 + workflow_status 九十六 | 无 |
| R2 | T8-G1：API Key 明文查看需 step-up | service/security_verification.go、middleware/secure_verification.go、router、web keys/chat/dashboard | ✅ 已闭环 | HTTP E2E：无证明 403 → 密码证明后 200；单测锁定 | 批量>100 已加前端上限提示（本批补） |
| R3 | T8-G4：邮箱枚举防护 | controller/misc.go SendEmailVerification | ✅ 已闭环 | TestSendEmailVerificationAntiEnumeration PASS（逐字节一致） | 无 |
| R4 | T8-G5：验证码存储 Redis 化 | common/verification.go | ✅ 已闭环 | TestVerificationRedisPath PASS（真实 Redis） | TTL=0 钳制已补（本批） |
| R5 | T15-A：订阅站点统计 | model/subscription_stats.go、controller/site_stats.go、/v1/stats/subscriptions、pricing 卡 | ✅ 已闭环 | TestSiteSubscriptionStatsAggregates PASS + 本地网关 E2E 200 | 无 |
| R6 | T15-B：通用 webhook 子系统 | operation_setting/webhook_setting.go、service/webhook.go、epay/task 接线、/webhook 设置页 | ✅ 已闭环 | service/webhook_test.go PASS + E2E 401/403 门禁 | task.settled 覆盖全任务路径已上移（本批） |
| R7 | T15-C：/v1/pricing 公开定价 | controller/pricing.go（既有 B5-4） | ✅ 复核确认 | E2E 200 | 无 |
| R8 | 真实 E2E 测验/验收/审计 | e2e-evidence/v1.3.46-*.json | ✅ 已闭环 | 本地网关二进制打点全过 | 浏览器级截图未做（见 R23） |
| R9 | 提交推送 main + tag + 发行版 | commit 7d6d2888、tag v1.3.46 | ✅ 提交/推送/tag 完成；⚠️ Release 创建受沙箱限制 | git ls-remote 确认远端 | 需在 Actions UI 触发 release.yml 或手工建 Release |
| R10 | 独立审查线程循环（六维度） | 本批主线程 + 独立 Critic 子代理 | 🔄 进行中 | 见修复清单轮次 | 等待 Critic 报告 → 修复 → 复验 |
| R11 | HTML 变更报告 + 底部测验 | 待产出 | ⏳ 未开始 | - | 全部修复后生成 |
| R12 | 项目整理为工作流 + 技能（供复用新 API/功能） | 待产出（.claude/skills/ + 工作流文档） | ⏳ 未开始 | - | 收尾时封装 |
| R13 | 需求追踪矩阵 | 本文档 | ✅ 已产出 | - | 随审查补充 |
| R14 | 全面盲点扫描（未知未知） | 本文档 + 修复清单 | 🔄 进行中 | - | Critic + 自我审计双通道 |

## 二、隐式需求追踪

| # | 隐式需求 | 状态 | 证据/动作 |
|---|---|---|---|
| I1 | 可运行/可构建（三库兼容） | ✅ | go build/vet/relaykit 独立构建全过；三库矩阵既有 CI |
| I2 | 可调用（一次调用跑通） | ✅ 核心链路 | E2E 全端点 200/403 符合预期；文档待补调用示例（R23） |
| I3 | 无伪实现/占位 | ✅ | 全部为真实实现 + 真实测试 + 真实 E2E |
| I4 | 文档同步（README/md） | ⚠️ 部分 | workflow_status/docs 已回填；README 主文档调用/部署示例待补 |
| I5 | 前后端契约对齐 | ✅ 本批 | 字段/枚举/错误码/鉴权头经 E2E 与单测锁定 |
| I6 | 主动补位（用户没提但关键） | ✅ 本批补 4 项 | task.settled 全覆盖、chat 取消死循环、webhook 保存 SSRF 校验、批量上限提示、TTL 钳制 |
| I7 | 安全/合规（OWASP） | ✅ G1-G5 | 重认证、防枚举、一次性消费、去敏；ASVS 参考已记录 |
| I8 | 异常路径可恢复 | ✅ 本批补 | chat 取消→重试；批量复制超限提示 |

## 三、非功能需求

| 项 | 状态 | 证据 |
|---|---|---|
| 三库兼容（SQLite/MySQL/PG） | ✅ | 全 GORM；聚合 SQL 简单 GROUP BY 跨方言；无新增表 |
| 并发/幂等 | ✅ | proof 一次性+上下文哈希；webhook 去重窗口；event bus 幂等 |
| 性能 | ✅ 本批 | 统计 COUNT 聚合；webhook 异步 gopool；无热点路径改动 |
| 可维护性 | ✅ | 复用 operation_setting 模式、event bus、SSRF 守卫；新增文件职责单一 |
| 可部署性 | ✅ | 配置走 operation_setting JSON，无新环境变量/新表 |

## 四、最强自我反驳（反向审判）

| 反驳点 | 问题本质 | 风险 | 现状 |
|---|---|---|---|
| 1. T15-B task.settled 只挂在批次轮询一处 | 视频任务/失败路径/其他平台结算不发 webhook → 功能「部分实现」 | P1 功能不完整 | ✅ 本批上移到统一 settleTaskBillingOnComplete，覆盖全部终态路径 |
| 2. chat 验证取消 → isPending 永久 true | 用户取消验证后被卡在「Preparing…」死循环，无恢复出口 | P1 UX 死循环 | ✅ 本批 revealFailed + retry + 错误分支重试按钮 |
| 3. webhook 设置保存时只查 scheme 前缀 | 可保存 `http://127.0.0.1` → 永不发送（静默失败） | P2 配置假可用 | ✅ 本批保存时 SSRF fail-fast |
| 4. G5 Redis TTL=0 → 永不过期 | 有效期误配 0 时验证码永不过期 | P2 安全隐患 | ✅ 本批钳制最小 1 分钟 |
| 5. 批量复制 >100 keys 后端 400 | 用户批量选择 >100 复制时一次静默失败 | P3 UX | ✅ 本批前端上限提示 |
| 6. 前端 /v1/stats/subscriptions 路径可能错 | 若 api 自动加 /api 前缀则 404（当时未验证） | P1 链路断 | ✅ 静态确认 api 不自动加前缀；E2E 200 证实 |
| 7. G1 前端 4 处披露路径可能遗漏调用点 | 若有其他 fetchTokenKey 调用未加 proof 则 403 回归 | P1 | ✅ typecheck+测试覆盖；全仓 grep 确认无遗漏 |
| 8. HTML 报告/测验、技能化封装未做 | 交付物缺失 | P2 | ⏳ 本批收尾执行 |
| 9. Release 创建受阻 | 沙箱无 gh/API/PAT | P2 外部阻塞 | 已推送 tag；需 Actions UI 触发 |
| 10. 浏览器级 UI 截图证据缺失 | 移动端/真实点击路径未截图 | P3 | 既有 v1.3.40 截图；本批新增 UI 未补截图，标注待验证 |
| 11. 覆盖度：新增 UI 组件无 axe 扫描 | a11y 门禁未覆盖新页面 | P3 | 记录为后续项 |
| 12. 三库矩阵本批未真跑（改动含 model 聚合查询） | 涉 DB 改动需三库验证 | P1 待验证 | 本批 model/subscription_stats 为只读 GORM 聚合，无 schema 变更；按纪律需跑一次 db-conformance |

## 五、待办（进入收尾）
1. 独立 Critic 修复清单 → 修复 → 复验（循环）。
2. ✅ 三库矩阵：`TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model/ -run TestDBConformance` **PASS（48s，7 测试 × sqlite/mysql/postgres）**（本批涉 DB 聚合查询已验证三库兼容）。
3. R23 文档：README 增 v1.3.46 变更/调用示例/新端点文档。
4. R11 HTML 报告 + 底部测验。
5. R12 技能/工作流封装 + 记忆更新。