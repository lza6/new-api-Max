# N6 侦察报告：Claude Code 生态与 skill 进化（已完成）

## 逐项目分析（摘要）

### 1. ECC（node|large）— agent harness 操作系统
- 286 skills + 94 commands + 30+ agents + 20+ rules 目录分层：skill=方法论，rules=规范，agents=角色，commands=入口
- 持续学习回路：Stop hook（scripts/hooks/evaluate-session.js）从 transcript 提取 pattern 写入 learned-skills；observe-runner 捕获所有工具调用；suggest-compact 每 ~50 次调用提醒压缩
- "Agentic OS" skill：kernel(CLAUDE.md)→agents→commands→scripts→data 五层目录契约，纯文件无数据库
- 评分：业务 4 / 架构 5 / 工程 5 / 迁移 4

### 2. obra__superpowers（node|large）
- Skill-Use 的 TDD 化（writing-skills/SKILL.md）：无 skill 基线（RED）→写 skill→复测（GREEN）→堵漏洞（REFACTOR）
- using-superpowers 元 skill 强制路由："哪怕 1% 可能适用也必须调用"；SessionStart hook 注入全文；Red Flags 表反驳 12 条合理化借口
- 多运行时兼容（13+ 运行时测试）
- 评分：业务 3 / 架构 5 / 工程 5 / 迁移 5

### 3. get-shit-done（node|large）
- **用户画像（最直接可迁移）**：templates/user-profile.md 8 维度（communication_style/decision_speed/explanation_depth 等），每维度 rating+confidence+directive+evidence；HIGH 直接照做、LOW 试探语气；agents/gsd-user-profiler.md 从会话打分
- 项目状态机：state.md 活记忆（phase/进度/velocity）；三档总结模板按上下文预算选档
- 编排：gsd-sdk query 一次性取全部 JSON、有界新近性、revision loop 上限 3 轮
- 评分：业务 4 / 架构 5 / 工程 4 / 迁移 5

### 4. claude-code-best-practice（node|medium）
- 概念卡片三重链接：官方文档+原理讲解+仓库实际文件（20+ 特性）
- Orchestration Workflow 教学：Command→Agent→Skill 三层编排 ASCII 图
- 评分：业务 2 / 架构 3 / 工程 4 / 迁移 3

### 5. shareAI-lab__learn-claude-code（doc|medium）
- 17 章从零实现 Claude Code：Harness=Tools+Knowledge+Observation+Action+Permissions 心智模型，章章递进
- s07 渐进披露最清晰教学：启动只放 name+description 目录进 system prompt，load_skill 时才注入全文
- s09 memory 四件套：存储/召回/提取/整理；"Memory agent 可写，Skill 人写只读"边界
- 评分：业务 3 / 架构 4 / 工程 4 / 迁移 5

### 6. Continuous-Claude-v3-main（doc|large）
- TLDR 5 层函数索引+1024 维嵌入，实测省 95% token（23,314→1,189）
- compound-learnings 闭环：gather→extract（固定 section 头）→频率表→判定固化 skill/rule/agent
- skill 激活注入：hook 注入"SKILL ACTIVATION CHECK"按规则表列 CRITICAL/RECOMMENDED
- 评分：业务 4 / 架构 4 / 工程 3 / 迁移 4

### 7. autoresearch（doc|medium）
- Metric/Verify 驱动 plan→loop→debug→fix→secure→ship；git 即记忆、失败自动回滚
- Orchestrator 元循环：目标→archetype→Success predicate→dry-run 验证→循环（Plateau 5/上限 50 停止）；ship 永远要求显式批准
- 评分：业务 3 / 架构 4 / 工程 4 / 迁移 4

### 8. beads（go|large）
- agent 工作原语：bd ready→claim→close→push/pull 多机同步；哈希 ID 防冲突；关系类型构成知识图
- 语义压缩：internal/compact/compactor.go 旧 issue 分 tier 快照+AI 摘要，保留审计轨迹
- bd remember/prime：持久记忆 + 会话开始注入上下文
- 评分：业务 3 / 架构 5 / 工程 5 / 迁移 4

### 9. spec-kit（node|large）— GitHub 官方 SDD
- constitution→specify→plan→tasks→implement→analyze→clarify→checklist→converge 十阶段命令管线
- 哲学：intent-driven、可执行规格、持续一致性校验、双向反馈、同规格多实现
- 评分：业务 3 / 架构 4 / 工程 5 / 迁移 3

### 10. OpenSpec（node|large）
- OPSX：schema.yaml+templates/*.md 用户可直接改、即时生效——"黑盒工作流"变"白盒工作流"
- goal→roadmap→slice→result 四层；spec.md 说"什么必须为真"、plan.md"怎么做"、result.md"实际发生了什么+证据"
- 评分：业务 3 / 架构 4 / 工程 4 / 迁移 4

### 11. claude-mem（node|large）
- 5 生命周期 hooks + SQLite + Chroma：SessionStart 起 worker→UserPromptSubmit 语义注入→PostToolUse 捕获观察→Summary 生成总结→SessionEnd
- 3 层检索省 token：search（~50-100 token/条）→timeline→get_observations（仅筛选后 ID 拉全文）
- worker 守护进程：本地 HTTP API（per-user 端口）
- 评分：业务 4 / 架构 5 / 工程 5 / 迁移 4

### 12. awesome-claude-code-main（python|large）
- 机器可读数据层（CSV）+ 多风格 README 自动生成 + 严格分类学
- 评分：业务 2 / 架构 2 / 工程 4 / 迁移 3

### 13. Claude-Code-Source-Study（node|medium）
- 34 章中文源码拆解：启动链路→6 条上下文压缩链路→Prompt Cache→工具协议→Agent→MCP→权限规则链→Hooks（27 事件）→三扩展点→IPC→Ink UI
- 第 21 章扩展点分层：Hook→Skill→Agent→Plugin 四档"行为面"+ Output Style"体验面"，字段级溯源
- 评分：业务 3 / 架构 5 / 工程 4 / 迁移 5

### 14. claude-code-workflows（node|medium）
- 三个审查工作流（code/security/design-review）slash command+CI 双形态；design-review 用 Playwright MCP 真实浏览器
- 评分：业务 2 / 架构 3 / 工程 4 / 迁移 4

### 15-16. book-to-skill / repo-to-skill（快速）
- book-to-skill：知识蒸馏为框架/决策规则/反模式/分章，24-51x token 节省
- repo-to-skill：clone→分析→生成 SKILL.md→装工具冒烟→with-skill vs baseline 评测闭环
- 评分：均业务 3 / 架构 3-4 / 工程 3-4 / 迁移 4

### 17. EvoSkill-main（python|medium）
- 五阶段进化环：Base Agent→Proposer（失败案例）→Generator→Evaluator（held-out 打分）→Frontier（git 分支版本化）
- feedback_history.md 记录每轮成败；跨 agent/模型/任务迁移有实验证据
- 评分：业务 4 / 架构 5 / 工程 4 / 迁移 4

### 18. SkillClaw（python|medium）— 形态与 new-api 最同构
- 双循环：任务循环 + 事后进化循环；Client Proxy 拦截 /v1/chat/completions 与 /v1/messages（对 OpenAI 兼容 API 透明）——拦截点与 new-api relay 层天然同构
- 进化服务端五件套：summarizer→aggregation→execution（固定 3 段或 agent 引擎）→skill_verifier（不确信就拒绝并记录）→session_judge
- 团队放大：多 agent/设备/用户喂同一进化环；idle 时才做后台验证；dashboard 查询
- 评分：业务 5 / 架构 5 / 工程 4 / 迁移 5

### 19. SkillOpt（python|large）— 微软研究院
- skill 当可训练参数：reflect.py=梯度、aggregate.py=聚合（失败驱动优先）；有界 add/delete/replace 编辑；held-out 严格提升才接受；文本学习率+拒绝缓冲+epoch 级更新
- SkillOpt-Sleep：harvest（本地只读收割 transcripts）→mine→replay→consolidate→stage proposal→用户采纳；gate_no_regression
- 52/52 best-or-tied
- 评分：业务 4 / 架构 5 / 工程 5 / 迁移 4

### 20. SkillSpector（node|large，NVIDIA）
- 71 漏洞模式/17 类（prompt injection/数据外传/提权/供应链/memory poisoning…）；两阶段（静态+LLM 语义）；AST+taint tracking+YARA
- fail-closed 资源上限防扫描器被打爆；OSV.dev CVE 实时+离线回退；SARIF 输出；baseline 假阳性抑制
- 数据：26.1% skill 有漏洞、5.2% 有恶意意图
- 评分：业务 3 / 架构 4 / 工程 5 / 迁移 4

## 汇总

### A. "agent 记住用户并越用越懂用户"最佳实践排序
1. **用户画像自动生成**（GSD user-profile）——落地成本最低、感知最强
2. **分层记忆+3 层检索**（claude-mem）——观察采集→后台摘要→向量库→渐进拉取
3. **会话证据→技能晋升闭环**（compound-learnings）——频率表→建议固化→人审生效
4. **经验→技能事后进化环**（SkillClaw 双循环）——适合平台化共享
5. **有界编辑+held-out 门禁**（SkillOpt）——自动改技能的安全护栏必抄
6. **记忆/任务衰减压缩**（beads compactor）——长期运营的记忆管理
7. **JSON/YAML 交接**（Continuous-Claude handoff）——简单高效

### B. 黑匣子透明化/教学化设计参考
1. Claude-Code-Source-Study 章节法（生命周期组织+file:line+可迁移模式）
2. shareAI-lab 四段章法（问题→解法→原理→可运行代码）+ SVG 原理图
3. claude-code-best-practice 三重链接卡片（文档+原理+实例）
4. claude-mem Web viewer（"AI 记了什么"直接展示）
5. OpenSpec OPSX 白盒工作流（模板可编辑即时生效）
6. book-to-skill 蒸馏文档（问什么加载哪章）

### C. skill 进化方法论对比
| 维度 | EvoSkill | SkillClaw | SkillOpt |
|---|---|---|---|
| 核心思路 | 失败案例驱动的进化搜索 | 真实会话事后消化+发布门禁 | skill 当可训练参数：梯度+有界编辑 |
| 数据来源 | 评测集 | 真实会话（proxy 拦截） | transcripts+重放 |
| 变更粒度 | 整文件重写 | 新建/合并/去重 | 有界 add/delete/replace |
| 质量保障 | held-out 打分 | 保守发布门禁 | held-out 严格提升+no-regression |
| 对 new-api 适配 | 有评测集时用 | **形态最同构：relay 层即拦截点** | 自动改技能的安全方法论 |

**结论**：三者是三层——SkillClaw=数据层（relay 拦截→摘要→聚合→发布门禁→共享，落地最快），EvoSkill=优化层（评测驱动搜索），SkillOpt=安全层（有界编辑+held-out 门禁，护栏必抄）。另两条：(1) skill 质量保障用 superpowers "skill TDD"；(2) 技能安装入口必须配 SkillSpector 式安全扫描（26.1% skill 有漏洞）。
