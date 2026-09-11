# N18 侦察报告：AI 应用平台与小白可用性（已完成，20 项）

## AI 平台"小白友好"设计模式 Top 清单（15 条分五类）

### A. 首次引导（Onboarding）
1. **动态分支向导**（MagesticAI getWizardSteps(provider) 按用户选择动态组装步骤，跳过无关环节）+ holaOS WorkspaceWizardLayout 统一布局原语（stepIndex/stepTotal/动作槽+错误条）
2. **全新实例专属流**（formbricks (fresh-instance) 路由组——空状态不报错而是进入引导）
3. **极简双步**（kaneo：workspace→success 两步够用，尊重 useReducedMotion）
4. **状态徽章先行**（MagesticAI AuthStatus/ClaudeCodeStatus/RateLimit 徽章——环境就绪与否一眼可见）

### B. 模板与示例
5. **首启动预装可执行模板**（langflow 20+ starter projects JSON 随安装注入；formbricks 模板库；wordpecker TemplateLibrary）——模板是可跑资产不是文档
6. **演示免登录**（DeerFlow allowlisted 只读 showcase——信任建立前不设注册墙）
7. **AI 代办安装**（cowart"把这段话发给 Codex"话术）

### C. 过程透明度
8. **时间轴回放**（LiveAgent trajectory 时间线+手势缩放）
9. **阶段折叠卡片**（LibreChat Agent activity phase cards+company-research-agent 按管道阶段分视图）
10. **拟人化包装**（holaOS employees AI=员工+头像——配置语言变雇佣语言）

### D. 错误恢复
11. **断线无感**（LiveAgent 有界 seq 窗口重放+DeerFlow offline banner 降级）
12. **部分成果保留**（LibreChat "Keep going / Answer now"）
13. **结构化错误码→人话**（astron CodeEnum 体系——"余额不足/密钥失效/限流"而非 stacktrace）

### E. 双路径与极简入口
14. **探索/目标双路径**（wordpecker Discovery vs Traditional Path）
15. **发布即双出口**（NexaFlow 同一资产发布成"公开分享页"或"API 文档页"）

## new-api 管理台最值得引入的 3 个
1. **动态分支 Onboarding 向导+状态徽章**（MagesticAI，迁移成本最低收益最直接）——新部署首访进统一向导：欢迎→选择接入方式（动态分支：添加渠道/OAuth/模板导入）→完成页给"下一步建议"
2. **结构化错误→人话映射**（astron+LibreChat）——错误码枚举体系，渠道列表和日志页映射为"密钥失效，请轮换"/"触发限流，已自动降权"；失败请求标注可重试性
3. **时间轴式日志回放**（LiveAgent trajectory）——日志页从表格升级为时间轴+可展开详情：时间→模型选择（为何选这个渠道）→重试事件→计费

次优先：首启动预装"应用模板"（langflow）；NexaFlow docs/WORKFLOW_DESIGN.md 作为工作流引入第一参考（中文、含 Dify/Coze/MaxKB 对照、边三态 UNKNOWN/TAKEN/SKIPPED 可纯函数实现）

## 关键项目补充要点
- LibreChat：Agent Marketplace 类应用商店交互；权限下沉前端组件层（useHasAccess 驱动 UI 显隐）；与 new-api 是"前端消费层 vs 后端网关层"天然互补（发 key vs 消费 key）
- langflow：starter projects 是首启动数据注入——"先跑通再理解"
- holaOS：Plugin Template SDK defineTemplate 一处声明生成模板元数据+onboarding 提示词+实例化事务
- astron-agent：plugin-store 独立页面（企业内"能力安装"变逛店）；DSL 引擎变量池+RetryConfig+错误码三件套
- copilotkit：channels 包族一次编写多 IM 渠道分发；AG-UI 协议标准化 agent 事件流
- LiveAgent：Go Gateway WebSocket+Protobuf 三链路+控制优先双队列写泵+拥塞掉帧——桌面 agent 远程化生产级协议
- NexaFlow：模型供应商引导式注册；上传三步向导；三语界面中文为键；发布双出口
- MagesticAI：动态分支向导+状态徽章
- formbricks：modules 按用户任务分域（前端组织映射用户旅程）
- DeerFlow：免登录只读 showcase；gateway-offline-banner 优雅降级
- kaneo：2 步引导内嵌 zod 校验+query 失效刷新
- company-research-agent：最复杂多 agent 管道入口是 3 字段表单；管道自带质检节点
- Auto-Company：consensus.md 接力棒模式（全部记忆=一个 Markdown）；单文件 dashboard 证明管理界面可极小成本存在
- cowart/agency-agents：一键安装话术；角色元数据标准化（emoji+vibe+color）
