# N19 侦察报告：垂直 Agent（交易/金融/科研/法律/医疗）（已完成，20 项）

## 关键项目

### HKUDS Vibe-Trading — 治理范本
- **RunManifest 治理指纹**（agent/src/governance/manifest.py）："这个数字怎么算出来的"内容寻址哈希——system prompt 存 hash、skill 存 (name, content_hash)、工具 registry 存名字表+hash、包版本钉死；timestamp 刻意排除使"方法论漂移"可 diff
- **策略工件验证状态机**：UNVALIDATED→IN_VALIDATION→VALIDATED→APPROVED 单向；adaptation 子策略强制 derived_from+证据清零
- **实盘 mandate 结构性信任不变量**：mandate 写入函数不注册为 agent 工具、agent loop 拿不到引用，唯一写路径需 API surface 的 consent_ack——"即使被注入也不能自我授权"；OrderGuard 五步 fail-closed
- 90 个金融 skill 分 9 类（alpha-zoo 决策树+工具表+约束+陷阱四段式）
- 评分：3/5/5/4

### hyperresearch — Deep Research 工程化天花板
- 16 步管线 skill 文件（入口薄路由，每步执行时才载入——防长管线上下文腐烂）
- **独立性审计**（core/independence.py）：URL 规范化/MinHash Jaccard≥0.7 近重复/通讯社 boilerplate 三路聚簇——"五家转载一篇通稿只算一个共识源"
- 对抗性四批评家并行+tool-locked patcher（只许 Read+Edit 物理禁止重写全文）+cite-check 幻觉引文硬阻断
- 付费墙论文走 Unpaywall 合法 OA 换取并三处披露；run manifest 断点恢复
- 评分：4/5/5/4

### MathModelAgent — 最适合整体模板化
- 6 阶段 skills 工作流（入口询问偏好→分析建模→编码→drawio→写作→verity 验收输出 VERIFY_REPORT.md）
- **HIL 6 种决策动作**（confirm/edit/regenerate/ask/skip/abort）——人审是枚举协议而非自由文本打断
- 四层容错：有限重试→Fallback Hand Off→Evaluator Shadow Mode→Feedback Rerun；阶段边界条款防越权
- 评分：3/3/4/4

### paper2code — 单 skill 即产品
- **歧义三分审计**：写代码前每个实现选择分类 SPECIFIED/PARTIALLY_SPECIFIED/UNSPECIFIED，未指定处 [UNSPECIFIED] 行内注释+替代方案清单——对抗 LLM 静默填补
- **引用锚定**：每行代码标注论文节与公式（# §3.2, Eq. 4）
- knowledge/（领域错误清单）+guardrails/ + worked example
- 评分：2/4/4/5

### AutoSciRub — 可迁移性最高（纯方法论零基础设施）
- 六个 JSON Schema 契约+criterion-level verification（修订由失败条目驱动）；"先归纳评估标准再干活"适用任何报告型垂直
- 评分：3/4/4/5

### claude-deep-research-skill
- 四档深度（quick/standard/deep/ultradeep）天然对应四档计费
- **claim/evidence schema**（claim_id=sha256(节+句)[:16]，只有 factual 硬失败于无支撑）+9 个验证脚本；验证循环最多 3 轮后停机报告
- 评分：3/4/4/5

### 其他
- AI-Research-SKILLs（MIT）：98 skills/23 类；双循环架构（INNER 假设实验快循环/OUTER 综合调整慢循环）+research-state.yaml；ARA 会话尾声记录器（任务结束后才扫描，"绝不污染工作上下文"）——**网关"研究助手"垂直的技能包货架本身**。4/4/4/5
- nautilus_trader：adapter 双层实现（性能层 Rust+管控层 Python）+venue 差异显式文档化。2/4/5/2
- QuantMind：**AGPL v3 不可入 new-api**；"每日质量回填告警"产品叙事可借鉴。3/3/4/2
- AutoHedge：反面教材（无风控门禁、私钥进环境变量）。2/1/2/2
- open-science/openscience：42 连接器 descriptor+独立测试+请求策略三件套；"能力未经 canary 不得标 verified"防虚假宣传。3/4/4/2-3
- OpenNutriTracker：**模型目录实测注释纪律**（"在 21 张照片上实测…"，"未实测不得写价格声明"成文规则）——直接适用于 new-api 模型管理/定价页文案规范。GPLv3 注意隔离。2/2/4/2
- 法律/医疗 skill 型：免责内建、危机分级；医疗对网关风险最高。1-2 分
- deer-flow/gsd：skill 生态运营形态参考

## 垂直 Agent 产品化共性
1. **领域知识注入按可靠性排序**：Schema 契约（最强）> 阶段化 pipeline（薄路由+按需载入+阶段边界条款）> 决策树 SKILL.md > 领域错误清单 > 连接器 descriptor
2. **输出交付物**：HTML/PDF 审计报告>结构化 JSON 证据包>可运行代码库>论文模板>策略代码——共性：报告都带"这个数字怎么来的"溯源
3. **人审节点**：结构化协议而非自由打断（6 种 HIL 动作/consent_ack 写路径隔离/机器可判 gate）；趋势：能自动化的验证先自动化，人只审机器判不了的

## new-api 首发两个垂直方向
1. **Deep Research 报告工厂**（hyperresearch+deep-research-skill）：纯 token 消耗与计费同构、档位即 SKU、零牌照风险、claim/evidence schema 质量可机检写进 SLA
2. **数学建模/论文生成**（MathModelAgent+paper2code）：竞赛学生付费意愿明确+时限压力；多角色分模型正好消费网关多模型路由；仅需 Code Interpreter 前置

**不首发**：交易下单（牌照）、医疗（PHI/监管）、法律（中文市场无数据面）
**许可证警示**：QuantMind AGPL v3 不可入；OpenNutriTracker GPLv3；主力可迁移项目（Vibe-Trading/hyperresearch/AutoSciRub/AI-Research-SKILLs）均 MIT；MathModelAgent/deep-research-skill 许可需确认
