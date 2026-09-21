# Plan：002 订阅套餐 + 智能流量单位 + 站点权威统计

## 阶段划分（依赖顺序）
- Phase A【规范/盘点】：本 spec + 现有订阅/流量代码盘点（已完成：Plan/UserSubscription/UpgradeGroup/支付网关已具备；缺周卡/档位/模型矩阵/覆盖/统计单位）
- Phase B【N1 智能流量单位 + 站点统计 + 日带宽排行】（独立、小、先落地）：
  - 后端 FormatBytes + 站点统计端点 + 排行端点（consume log 聚合，GORM，三库兼容）+ 单测
  - 前端共享 formatTraffic util + 首页/关于页站点统计卡 + 管理端带宽排行页 + i18n
- Phase C【N2 订阅档位 + 模型矩阵 + 覆盖】（依赖 A）：
  - SubscriptionPlan 增列（models/concurrency/rpm）+ UserSubscription 增列（rpm/concurrency override）+ 三库迁移
  - 订阅限流执行（并发/RPM → 429）接入既有 middleware；模型矩阵校验接入 relay 入口
  - 管理端订阅管理 API + 前端（套餐管理/用户订阅/档位覆盖）+ i18n
  - deepseek-v4-flash 入卡种子 + 订阅说明微信 Tf00798 文案
- Phase D【发行版泄漏排查 + 文档/审计】：
  - 扫描仓库/Release 描述中的上游地址/密钥；整改登记
  - workflow_status.md 登记；audit-ledger.md 记测试范围；README 同步
- Phase E【总验收/交付】：
  - 后端测试（controller/service/model 相关包）+ 前端 typecheck/build/vitest/i18n
  - 三库 conformance 回归；主题 commit → push → tag → Release → 核验远端

## 依赖关系
- B 独立（可先行）；C 依赖 A 盘点；D 与 B/C 并行收尾；E 依赖 B/C/D

## 并行机会
- Phase B 与 Phase C 后端建模可并行（不同文件：log_traffic/log.go vs subscription.go）
- Phase D 扫描与 B/C 实现并行

## 关键设计决策
- 档位执行：订阅限流放在既有请求级限流链（middleware）之后、relay 入口之前；按 user_id 的订阅档位查一次缓存，并发用内存计数，RPM 用滑动窗口/计数器（复用既有 quota 限流基础设施时取最严）
- 单位换算：统一 1024 进制、2 位小数；后端 common.FormatBytes 与前端 formatTraffic 各自单测，契约一致
- 站点统计：避免全表扫描——consume log 已按日索引；统计端点默认聚合最近 N 天 + 可累计缓存（首版直接聚合，百万级走 EXPLAIN 验证）
- 模型矩阵：Plan.Models 存 JSON 数组；校验函数在 relay 入口对订阅用户生效；未订阅用户不受影响
