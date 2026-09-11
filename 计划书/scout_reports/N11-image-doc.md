# N11 侦察报告：图像生成/设计/PPT文档生成（已完成，20 项目）

## 关键项目

### baoyu-skills（宝玉技能集）★ 5/5/5/5 本批最高
- 20+ 内容生成 skill；**生图后端解析优先级链**（≈生图版 model routing）：请求指定 > EXTEND.md 偏好 > 运行时原生工具 > codex CLI 包装 > 已装 skill > 询问用户；禁止 SVG/HTML 渲染冒充位图生成
- **codex-imagegen 后端工程化**：验证 image_gen 真被调用（检查生成目录而非信任返回值）、PNG magic-byte 校验、prompt+aspect+refs 幂等缓存、文件锁防并发、JSONL 结构化日志、9 类 error_kind 重试分类——"生成可靠性层"可直接搬到 new-api 生图渠道适配器
- **提示词文件先行**：生成前完整 prompt 写到 prompts/NN-{type}-[slug].md，后端只收文件——可复现、可换后端、审计凭据
- 21 layouts × 22 styles 二维组合

### freestylefly__awesome-gpt-image-2 ★ 5/5/4/5
- GPT-Image2 "Prompt as Code" 工业级提示词引擎+544 逆向案例，已长成完整 SaaS——本批离"new-api+模板市场"最近的成品
- **机器可读风格库真源**：data/style-library.json（version/双语标签/模板/场景/避坑），skill 文档由 npm run generate:style-skill 从 JSON 生成——JSON 真源→生成 skill 文档单向数据流
- **完整计费闭环**：Supabase RPC reserve_generation_usage 生成前额度预留（失败抛 CREDITS_REQUIRED）→上游 APIMart（价格快照+fallbackPricing 降级）→Stripe/支付宝 webhook→pricing API 缓存——"预留额度→上游生成→回调结算"正是 new-api 任务计费参考实现

### dashi-ppt-skill 5/4/4/4
- 12 主题×1020 版式×8576 控件；**每页 3 模板方案+1 bespoke 方案**（前三套锁模板填文案，第四套 agent 定制）——2×2 对比大幅降低返工
- JSON goal-spec 契约（title/goal/audience/pageCount/themePack）交本地 Node 生成器；**产物即编辑器**（每页自带滑杆/开关/点击改字/拖拽换图，导出保持文字可编辑）
- 按页计费（pageCount 在 goal-spec 里）天然成立；token 消耗低

### EpicInfographics 4/5/4/4
- 12 设计语言×6 画布 preset 正交参数空间
- **机械预检门**：check.mjs 用 headless 浏览器实测字形几何（文字覆盖/裁切/可读下限/data-hero 唯一性），error 不清零不出图——"审美规则编译成可执行断言"典范
- **check-render-review 闭环**：渲染后强制回看 PNG 过 anti-slop 清单（"换别的数据集还成立吗=模板货"）
- charts.md 要求图表算术显式化（radius∝√value 写进注释）禁目测比例

### 其他
- Infographic（AntV）：AI 流式输出 DSL 直接渲染，~200 模板；模板前缀路由主数据字段；模板变更 4 处同步清单写成 skill。4/5/5/4
- ppt-master：四类正交模板规则束（Brand/Style/Layout/Deck 独立目录+design_spec frontmatter，读取时派生路由）；SVG→原生 PPTX 逐特性映射+SmartArt"有意的省略"诚实披露；**README 挂 5 家 API 中转商赞助位——skill 型项目靠 API 中转分佣变现活例**。5/5/4/4
- ppt-agent-skills：主 agent 零生产红线（P0→P5 固定链，正式产物必须由 subagent 生成）；资源双层消费（planning 只加载标题层，html 阶段按 JSON 字段路由加载正文层——token 经济懒加载）。4/5/4/3
- abi__screenshot-to-code：工具化 agent 引擎（create_file/edit_file/screenshot_preview 强制自查再修——render→look→fix 闭环）；多供应商按能力分工（Gemini 资产抽取/Replicate 生图/OpenAI codegen）。3/4/5/3
- handraw-style：216 种编号风格库；**模型能力感知的参考图决策**（按 model_capabilities.json：作者名激活强→只发名字；特征强→名字+视觉特征；都不行→才传参考图）——"按用户所选模型动态降级生成策略"样板。4/4/4/4
- hand-drawn-explainer-video-nikola：分层不串扰（换画风不换声音，重做一幕不重生成已通过旁白）；交付前验证链+contact sheet 证据。4/4/4/3
- ian-xiaohei-illustrations：固定 IP（小黑）+风格 DNA+动作库+构图模式+QA 清单五 reference 按需加载——**IP 化风格=独占性模板资产**。4/3/3/4
- img2threejs：五阶段状态机+**配额化循环**（passCount/maxPerPass/STOP:stopReason——agent 无需自己决定流程）；确定性脚本（近乎免费）+LLM 只做判断（贵）；**token 成本表写进文档**（80k-180k/物体）。2/5/5/3
- OJO-Design-Skills：五拨盘 register 推导（每取值必须引用证据）；模糊词防火墙（"高级感"必须翻译成可观测决策）。3/4/4/4
- mono-color-skill：**设计目录(JSON)+manifest(参数 schema)+evals(质检)=一个风格模板的全部要素**。3/4/4/5
- app-store-screenshots：JSON deck 作状态真源（git 可追踪）；"截图是广告不是文档"原则；任务模板=可运行应用模板。3/4/4/3
- ai-image-prompts-skill：15,586 条真实 prompt 库（12 分类 JSON+manifest 驱动分发+每日同步）；"每条推荐必须带样图否则整条跳过"硬规则——prompt 库即商品目录。4/3/3/4
- kids-blessing-video-wizard：零 API 成本模式（skill 出提示词+指导用户手动操作）；**"生成任务提示词包"可作免费引流/低价 SKU**；六步向导每步停等确认是对话式任务模板交互范式。3/3/2/4
- acgti-anime-persona-quiz：测试→分享海报裂变链路。2/2/3/3
- OfficeCLI：L1 读→L2 DOM 编辑→L3 XML 分层策略；内置渲染引擎做 render→look→fix（"给 AI 眼睛"）——可作 PPT/Word 任务导出执行器与质检器
- gova-declarative-gui：参考根目录不存在（只有无关 govctl/）

## 四条共性机制
1. **模板化=参数空间显式化**：风格拆成正交参数而非整段 prompt；模板真实结构=(输入契约 schema)+(参数空间)+(管线脚本)+(质检断言) 的打包
2. **质量闭环是可执行断言，不是口头审美**：check.mjs 几何预检/anti-slop 清单/双保险/渲染-审查环配额/contact-sheet——**"质检失败=不计费"技术上完全可行**
3. **后端无关的生成分发层成共识**：backend resolution chain、能力感知三级降级、多供应商按能力分工——同一任务模板必须能根据渠道/模型能力自动调整生成策略
4. **计费与审计内建在管线里**：reserve_generation_usage 额度预留、prompt 文件先行（审计凭据）、幂等缓存、验证生图真发生、token 成本表

## 对 new-api 落地建议（按优先级）
1. **任务模板数据模型**：JSON 真源（参数 schema+输出契约+计费单元=按张/按页/按秒）→生成面向用户与 agent 两套文档（generate:style-skill 单向数据流）
2. **计费单元**：图像按张（预留额度→生成→回调结算）；PPT 按页；视频按秒/按段
3. **服务端验收器**：check.mjs 式几何预检+anti-slop 清单做成生成后自动验收，通过才结算，失败自动重试不扣费
4. **首批模板 SKU**：信息图（EpicInfographics/AntV DSL 两路线）、PPT（dashi-ppt 锁模板填文案路线 token 成本最低）、文章配图（小黑 IP 型独占风格）、风格库（handraw-style 216 风格直接入库）
