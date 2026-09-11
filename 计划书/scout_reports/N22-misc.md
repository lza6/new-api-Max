# N22 侦察报告：漏网之鱼扫尾（已完成）

## 指定点名项目核查（20 个全部定位）

| 名字 | 状态 | 结论 |
|---|---|---|
| Pake | node, medium | 有借鉴点（agent CLI 契约） |
| tabby | node, large | 边缘价值 |
| hyperdx | node, large | 有借鉴点 |
| formbricks | node, large | 边缘价值 |
| claude-devtools | node, large | 高价值 |
| claude-hud-main | medium | 小而精 |
| cc-switch | node, large | 高价值 |
| claude-octopus | skill 型, large | 有借鉴点 |
| Jellyfish / Forget-C__Jellyfish | 两者代码相同，取一 | 有借鉴点 |
| Hermes-CN-Desktop | node, large | 有借鉴点（运营模式） |
| Aether-desktop-orchestrator | 纯落地页 | 无价值 |
| dataset-viewer | node, medium | 有借鉴点 |
| unsloth | python, large | 低价值 |
| MinerU | python, large | 低-中价值 |
| flash-attention-main | CUDA 内核 | 无直接价值 |
| dagucloud__dagu | go, large | 有借鉴点 |
| daytona | doc/small（N17 已覆盖） | 已覆盖 |
| mirrord | node, large | 边缘价值 |
| cloudflare_temp_email | skill 型 | 低价值 |
| tgDrive | java, medium | 低价值 |
| MasterDnsVPN | go, medium | 边缘价值 |
| superset-main | 是"AI Agent 代码编辑器"非 Apache Superset | 已被 N20 类主题覆盖 |
| JeecgBoot | doc-or-mixed, large | 边缘价值 |
| PeerTube | node, large | 无直接价值 |
| grt | 不存在 | — |

## 有真实价值的漏网项目

### cc-switch [Tauri 桌面工具]
- 定位：Claude Code/Codex/Gemini CLI/OpenCode 等 8+ AI 编码工具的统一配置切换器
- 亮点：50+ 内置 provider 预设（README.md"import providers with one click…50+ built-in provider presets, unified MCP and Skills management…SQLite database with atomic writes"）；Tauri 2 + SQLite 原子写；被 Kimi/PackyCode/ZetaAPI 等 API 中转商赞助（与公益服务网关同批潜在赞助方）
- 可借鉴：**对 new-api 价值最高**——目标用户正是在各 CLI 工具里配置中转端点的这群人；provider 预设清单、各工具配置格式知识可转化为 new-api 的"一键接入文档/预设下载"功能，也可作自建配套切换器蓝本
- 评分：业务 4 / 架构 2 / 工程 4 / 迁移 4

### claude-devtools [Electron 调试工具]
- 定位：读取 ~/.claude/ 会话日志，重建 Claude Code 被折叠输出的完整过程（工具调用/diff/思考链/token 归因）
- 亮点："Per-turn token attribution across 7 categories with compaction visualization"（README.md）；跨平台分发 + Homebrew
- 可借鉴：new-api 日志/消费页面可借鉴其 token 分类归因和"折叠→展开重建"展示模型；解析 CC session JSONL 的代码可迁移
- 评分：业务 3 / 架构 3 / 工程 4 / 迁移 4

### dagucloud__dagu [Go 工作流引擎]
- 定位：local-first 单二进制 DAG 工作流引擎（YAML 声明、重试、人工任务、Web UI、运行历史）
- 亮点：单二进制零外部依赖；**内置 MCP server** 可检查/修改/控制工作流与维护 Wiki（README.md）；cron+时区+补跑窗口
- 可借鉴：new-api 的渠道定时测试、任务队列、日志清理等内部作业可参考"YAML 即工作流+运行历史+重试"模型；MCP server 暴露内部运维的思路对 new-api 做 MCP 管理面有直接参考
- 评分：业务 2 / 架构 4 / 工程 5 / 迁移 3

### hyperdx [可观测性平台]
- 定位：ClickHouse 之上的日志/追踪/会话回放统一搜索（"Kibana for ClickHouse"）
- 亮点：原生 MCP server 让 AI 助手查询观测数据（MCP.md，22 处 MCP 引用）；自然语言式属性搜索语法 level:err；OTel 开箱即用
- 可借鉴：用量/日志排障 UX 可借鉴其全文+属性混合搜索语法；"管理员用自然语言查消费异常"现成范式
- 评分：业务 2 / 架构 4 / 工程 5 / 迁移 3

### llmquota [Node TUI]
- 定位：聚合本机 Claude Code/Codex/Cursor/Grok/Hermes 的限额状态到"竞技场"TUI
- 亮点：统一读取各 CLI 各不相同的限额口径（README.md），who/hop/usage 一行式输出
- 可借鉴：各厂商配额读取器实现清单对 new-api 的"用户多渠道余额/限额聚合展示"是现成参考
- 评分：业务 4 / 架构 2 / 工程 3 / 迁移 4

### Jellyfish [FastAPI+React 全栈]
- 定位：AI 短剧端到端生产工作台（剧本→分镜→一致性资产→图/视频生成→导出）
- 亮点：一致性（角色/场景/道具/服装）作为一等资产管理；长耗时生成统一走异步任务系统（状态/取消/恢复）（README.md Core Value + backend/README.md：FastAPI+LangGraph+异步 ORM）
- 可借鉴：对 new-api 异步任务域（视频/图片渠道长任务状态机：status/cancel/recovery）有直接参考；资产一致性模型独有
- 评分：业务 2 / 架构 3 / 工程 4 / 迁移 3

### nezha [Tauri 轻量 IDE]
- 定位：7MB 跨平台轻量 IDE（多项目工作区、会话自动发现、Git Worktree、Skill 管理、内嵌终端直连 CC/Codex）
- 亮点："Agent 优先设计"产品论证 + 会话自动发现 + 任务生命周期追踪（README.md）；安装包仅 7M
- 评分：业务 2 / 架构 3 / 工程 4 / 迁移 3

### Pake [Tauri 工具]
- 定位：一条命令把网页打包成桌面 App（体积比 Electron 小 20 倍）
- 亮点：**面向 AI agent 的 CLI 契约设计**：--json 机器可读输出、--config JSON Schema、llms.txt agent 手册、官方 Claude Code skill（README.md"Using Pake from a script or AI agent?…See llms.txt"）
- 可借鉴：new-api 的 CLI/安装脚本可复用这套"agent 可用性契约"模式
- 评分：业务 2 / 架构 3 / 工程 4 / 迁移 4

### claude-octopus [CC 插件框架]
- 定位：给 Claude Code 挂 12 个外部模型做"共识门"（75% 共识门拦截分歧再上线）
- 亮点：31 personas / 53 commands / 63 skills 显式分层（README.md、agents/、commands/）
- 可借鉴：多模型对抗式审查"门控"模式 → new-api"多渠道结果一致性校验/仲裁"产品化思路
- 评分：业务 2 / 架构 3 / 工程 3 / 迁移 3

### Hermes-CN-Desktop [Tauri 桌面客户端]
- 定位：Hermes Agent 中文社区桌面端（Tauri v2 + Rust + React）
- 亮点：完整社区化分发体系（官网/文档站/微信群/赞助商推广位）（README.md）；e2e 目录 + installer 打包链
- 可借鉴：**商业模式参考大于代码价值**——社区桌面端+国内 API 厂商赞助位与公益服务网关运营形态高度同构
- 评分：业务 3 / 架构 2 / 工程 3 / 迁移 2

### dataset-viewer [Tauri 数据查看器]
- 定位：100GB+ 数据集流式查看器（虚拟化渲染、毫秒搜索、免解压预览）
- 亮点：WebDAV/SSH/SMB/S3/HF Hub 多协议 + 全格式虚拟滚动（README.md）
- 可借鉴：管理后台查看大日志导出/备份的流式读取与免解压预览现成方案
- 评分：业务 1 / 架构 3 / 工程 4 / 迁移 3

### 二线参考
- VaultS3（go）：17MB 单二进制 S3 兼容存储带仪表盘
- mirrord（rust）：本地进程注入 K8s 上下文，官方支持 Claude Code
- tabby（rust）：crates 划分是 Rust 大型服务工程范本
- JeecgBoot（java）：AI 低代码平台流程编排与权限颗粒度
- formbricks：问卷/XM，用户反馈收集
- claude-hud-main：状态栏 HUD 插件（上下文用量/agent/todo 常显），模式小而好抄

## 低价值/无价值排除清单（摘要）

- 纯落地页/内容仓库：Aether-desktop-orchestrator、web-check、github-readme-stats、developer-roadmap、system-design-101、docus
- 通用基础件：htmx、uv、opentofu、greptimedb、dragonfly（可作 Redis 备选）、carbon-lang、flash-attention
- 训练侧：unsloth（微调）、MinerU（PDF 解析，仅文档渠道时有用）
- 已覆盖主题的重复克隆：约 60-80 个 agent harness 复用器（hapi/WrongStack/VetarAI/Kun/Orkas 等已被 N5/N6/N17/N18/N20 覆盖）、约 20 个网关克隆（N2 覆盖）、媒体/渗透/交易类约 40-50 个

## 全量 1015 项目价值分布估计

- **完全无参考价值：约 35-42%（350-430 个）**——skill/doc 型合计 38% 多为技能包集合与 awesome 清单，加上 no-readme、内容仓库、通用基础件、纯重复克隆
- **边缘/单点可借鉴：约 25-30%**
- **真正高价值：约 20-25%**，绝大部分已被 N1-N21 挖掘；本次扫尾新增高价值漏网约 10 个（cc-switch/claude-devtools/dagu/hyperdx/llmquota/Pake/Jellyfish/nezha/claude-octopus/dataset-viewer）
