# N21 侦察报告：Skills 生态分类评估（已完成）

## 分类表（188 个 skill 型项目）

| 类别 | 数量(约) | 代表项目 |
|------|---------|---------|
| 安全/渗透/逆向 | ~25 | Anthropic-Cybersecurity-Skills(818子skill)、Claude-BugHunter(83个hunt-*)、PE-reverse-skill、ghidra-re-skill |
| 设计/前端/UX | ~20 | awesome-ux-skills、taste-skill、impeccable-main、awesome-design-skills(67个) |
| 视频/媒体创作 | ~30 | ffmpeg-skill、claude-video、video-shotcraft、OpenMontage、hand-drawn-explainer |
| PPT/图像/文档 | ~12 | dashi-ppt-skill、ppt-master、baoyu-skills(22个)、EpicInfographics |
| 研究/知识蒸馏 | ~15 | claude-deep-research-skill、scientific-agent-skills(163个)、cangjie-skill、book-to-skill |
| 营销/SEO/广告 | ~8 | claude-ads(12平台+52skill)、claude-seo、marketing-skills(76个) |
| Skill 元工程 | ~10 | nuwa-skill、darwin-skill-master、repo-to-skill、claude-md-doctor |
| Agent 平台全家桶 | ~35 | superpowers、everything-claude-code-main(457个)、agentic-awesome-skills(6450个!) |
| 记忆/上下文工程 | ~12 | mem0、supermemory、ownmem、mnemosyne |
| 办公/生活/其他 | ~20 | googleworkspace__cli、chrome-cdp-skill |

## Skill 工程标准形态（已验证）

- **格式已收敛**：SKILL.md + YAML frontmatter；抽样 1000 个：100% 有 frontmatter，name+description 100% 覆盖；description 是激活唯一路由依据；最佳实践=做什么+何时用+触发词
- **扩展字段分层**：注册表附加（risk 三级/source/license/tags）；领域附加（Anthropic 网安用 domain/atlas_techniques/nist_csf；Claude-BugHunter 用 sources/report_count）
- **物理形态配比**（1934 抽样）：63% 纯 SKILL.md 单文件；24% 带 references/；仅 4.9% 带 scripts/；p50≈8KB——大量 skill 是"长 prompt"而非"程序"
- **渐进披露教科书——ffmpeg-skill**：335 行 SKILL.md 只写工作流与边界；42 脚本（全支持 --dry-run/--json/--help）；细节下沉 references/；带 evals/；"职责分界"声明不做什么
- **高风险长任务范式——js-reverse-skill**：SKILL.md 内嵌状态机协议（--init/--set/--guard 强制推进 11 节点、GATE-0~2 硬门禁）——SKILL.md 即协议，脚本即执法
- **元生态**：nuwa(造人：蒸馏思维框架成 skill)→darwin(质检：8 维 rubric+爬山+git 棘轮+独立子代理评分)→cangjie(拆书成原子 skill 集)，三者互引构成"生产→质检→优化"

## 安全问题观察

1. 4.9% skill 带可执行脚本，安装即入 agent 可调用范围；agent-skills-hub 扫描器（SlowMist 11 类红旗：curl|bash、凭据收割、~/.ssh 访问、base64 解码执行）证明真实存在
2. references/*.md 是注入载荷天然藏身处；description 触发词可被毒化做路由劫持
3. risk 字段多为自报；市场止步于"正则扫描+LLM 二审"，无运行时行为审计
4. 大量 skill 要求用户提供第三方 API key，key 存放/代理完全靠用户自觉

## 市场产品对比

| 产品 | 形态 | 差异点 |
|------|------|-------|
| skills.sh（事实标准） | 注册表+安装计数 | npx skills add；各家全对接其 API |
| agent-skills-hub | 网站目录 | 117k 索引；安全分级是卖点（正则→LLM 复审→trust tier）；只索引不托管 |
| agentfiles | Obsidian 插件 | 本地管理 17 个 coding agent；内嵌 skills.sh；路径遍历防护 |
| skills-manager | Tauri 桌面 | 中央库+preset 组+双工作区+备份同步 |
| SkillX-main | 学术管线（zjunlp） | 轨迹自动构建三层 skill 知识库，弱模型插库提升 |

格局：**分发层已收敛；目录站靠安全分级差异化；本地管理器红海；无人做"托管+沙箱验证+计费"闭环市场。**

## 对 new-api 的意义

1. **格式接入成本极低**：SKILL.md 最小规范即可托管/展示/安装
2. **差异化空间在"验证与运行"**：现有市场止步静态扫描；new-api 网关天然有计费/配额/密钥代理——skill 运行依赖（模型调用/key）正是网关主场。闭环：skill 市场+运行时沙箱+模型调用走网关计费+key 平台托管
3. **安全审查是必答题**：上架至少复制 agent-skills-hub 正则规则库（MIT），进阶 LLM 语义复审+沙箱试运行（ffmpeg-skill 三件套是上架范本）
4. **中文生态活跃且本土市场空缺**：nuwa/darwin/cangjie/hand-drawn 等中文原生 skill 质量高传播强
5. **风险**：skill 仓库生命周期短、重复收录多（fork/-main 副本），需 repo_full_name 去重

## 整体评分：4.5/5

关键文件：ffmpeg-skill\SKILL.md+evals\ · js-reverse-skill\SKILL.md · nuwa-skill\SKILL.md+references\skill-template.md · darwin-skill-master\SKILL.md · agent-skills-hub\backend\app\services\security_scanner.py · agentfiles\src\marketplace.ts
