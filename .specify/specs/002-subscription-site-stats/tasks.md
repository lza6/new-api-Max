# Tasks：002 订阅 + 流量单位 + 站点统计

## Phase B：智能流量单位 / 站点统计 / 日带宽排行
- [ ] B1 后端 FormatBytes 工具 + 单测（B/KB/MB/GB/TB，1024，2 位小数，0/负数边界）
- [ ] B2 站点权威统计端点（total_bytes/total_requests/total_tokens，consume log 聚合）+ 单测 + EXPLAIN 抽查
- [ ] B3 每日带宽消耗排行端点（按日分组降序，分页）+ 单测
- [ ] B4 前端共享 formatTraffic util + 单测（vitest）
- [ ] B5 首页/关于页站点统计卡 + 管理端带宽排行 UI + i18n 全语言
- [ ] B6 三库 conformance 回归（聚合查询路径）

## Phase C：订阅档位 / 模型矩阵 / 覆盖
- [ ] C1 SubscriptionPlan 增列 Models/ConcurrencyLimit/RpmLimit + UserSubscription 增列 RpmOverride/ConcurrencyOverride + AutoMigrate 三库
- [ ] C2 周卡时长单位 week（7*24h）计算 + 单测
- [ ] C3 订阅档位限流执行（并发 3→429、RPM 150→429）接入 middleware + 单测（并发/RPM 超限、未订阅不受影响、覆盖优先）
- [ ] C4 模型×套餐访问矩阵校验 + 单测（放行/拒绝/可读错误）
- [ ] C5 管理端订阅管理 API（用户订阅列表/开通/续费/停用/档位覆盖 + audit）+ 单测
- [ ] C6 deepseek-v4-flash 入天/周/月无限卡种子 + 订阅说明微信 Tf00798 文案（前后端 i18n）
- [ ] C7 前端订阅页/套餐管理/用户订阅管理 UI + i18n

## Phase D：泄漏排查 / 文档 / 审计
- [ ] D1 仓库+Release 描述上游地址/密钥扫描，产出清单 + 整改 + 登记
- [ ] D2 workflow_status.md 登记；audit-ledger.md 记录测试范围；README 同步
- [ ] D3 HTML 变更报告 + 底部测验

## Phase E：总验收/交付
- [ ] E1 后端相关包测试全绿；前端 typecheck/build/vitest/i18n 全绿
- [ ] E2 三库 conformance 通过
- [ ] E3 主题 commit → push → tag → Release → 远端核验

## Phase B-2：系统信息资源显示 + 模型广场卡片 + 效果测试整合（2026-09-22 补充）
- [ ] B2-1 系统信息资源恒采样（CPU/内存/磁盘与性能监控开关解耦）— ✅ 已实现 common/system_monitor.go
- [ ] B2-2 模型广场每卡片统计：今日调用总数/成功数、近30天总数/成功数（后端聚合端点 + 前端卡片）
- [ ] B2-3 模型效果测试整合到模型广场：模型卡片下展示效果测试（模型名/测试时间/输入输出），复用 routes/model-test.tsx 资产
- [ ] B2-4 效果测试数据可配置化（当前 SITE_MODEL/TEST_TIME 硬编码在 model-test.tsx）
