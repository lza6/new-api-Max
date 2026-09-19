# Plan：终局审计闭环

## 阶段划分（依赖顺序）
- Phase A【盘点/只读】：4 个并行审计节点 → 状态矩阵 + 发现清单（R1-R4）
- Phase B【P0/P1 修复】：用户已报问题（S1 会话上限、S2 模型测试入口、S3 封禁 IP、S4 签到报错）+ 审计发现的阻断缺陷
- Phase C【P1/P2 补位】：错误人话/onboarding/hover-why 收尾、契约测试、慢查询索引、计费安全回归
- Phase D【文档/运维】：SOP、audit-ledger、可观测性说明、观察性端点补强
- Phase E【总验收/交付】：质量门全绿（存量失败基线对照）→ 主题 commit → tag/Release → 生产部署 + 线上验收

## 依赖关系
- B 依赖 A（发现清单）
- C 依赖 A+B（部分）
- D 与 B/C 并行收尾
- E 依赖 B/C/D

## 并行机会
- Phase A 四个节点完全并行（互不写同一文件，输出各自 audit/ 文档）
- Phase B 中 S1/S2/S3/S4 可并行（不同模块）
