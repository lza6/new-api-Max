# 游乐场重做 — 任务台账（2026-10-13）

> 目标：把游乐场做成参考项目（`自营的SaaS服务平台`）那种**完整的对话式 AI 工作台**。
> 纪律：只记录实际运行的命令与观察到的结果；未验证项明确标「待验证」。

## 0. 现状侦察（读码 + 参考项目对照，均为实测）

| 能力 | 现状 | 证据 |
|---|---|---|
| 会话列表侧边栏 | ❌ 只有顶部一个 `<Select>` 下拉 | `components/header/conversation-switcher.tsx`（101 行，Select 实现） |
| 按日期分组（今天/昨天/7天） | ❌ 不存在 | `grep -rn "今天\|Yesterday\|groupBy"` 零命中 |
| 侧边栏折叠 | ❌ 不存在 | 同上 |
| 新建会话 | ◐ 有按钮但在下拉旁，不显眼 | `conversation-switcher.tsx` 的 `onCreate` |
| 模型选择分类（文本/图片/视频） | ❌ 单一列表 | 用户截图：一个 popover 平铺模型 + `default` 分组徽章 |
| 图片渲染 | ✅ markdown 图片会渲染 | `response-renderer-image.tsx`（含 `onError` 降级） |
| 图片**下载按钮** | ❌ 无（只有视频有） | `grep -rn download` 只命中 `video-result-panel.tsx:134` |
| 图片**点击放大** | ❌ 无 | 无 lightbox |
| 上传参考图 | ✅ 有（走 `usePromptInputAttachments`） | `playground-input-tools.tsx:66` |
| 上传**数量上限**提示 | ❓ 待验证 | 需实测 |
| 拖拽上传 | ❓ 待验证 | 需实测 |
| **用历史图片做二次改图** | ❌ 无「图生图」入口 | 参考项目 `agent-chat-result.js:113-119` 有 `🖼 图生图` |
| **Agent 提问给选项（ask_options）** | ❌ 不存在 | `grep -rn "ask_option\|AskUser"` 零命中；参考项目 `agent-tools.js:37-52` |
| **画廊** | ❌ 不存在 | `grep -rln gallery src/features/` 零命中 |
| 分辨率/比例选择 | ◐ 有参数面板 | `playground-parameter-panel.tsx`（411 行）；需验证是否含「自动比例」 |

## 1. 参考项目的可借鉴设计（已读码确认）

- **会话侧边栏**：日期分组 `今天 / 7 天内 / 30 天内 / 更早`（注意：参考项目**没有**「昨天」桶）；
  置顶分组 `📌 置顶`；每项 hover 出 `📌 置顶 / 📥 归档 / ✕ 删除`；
  折叠状态持久化 `localStorage`；折叠时露出「新建」小按钮。
- **ask_options**：作为**一等公民工具**存在于 tool schema（`question` + `options[2-5]` + `extra`
  是否允许自由输入）。前端渲染为选项卡片，支持**数字键 1-9 快选**、`✏️ 输入我的想法`、
  `🔄 换一批选项`，并带 **25 分钟 TTL**（过期则禁用 + 提示重新描述）。
- **画廊**：`GET /api/chat/sessions/:id/images` 从每条消息的 `meta.taskImages` 抽取并按
  `taskId || (url + '|' + prompt)` 去重；CSS Grid `repeat(auto-fill, minmax(96px,1fr))`；
  **注意**：参考项目点缩略图是开 lightbox，并**不**跳回原消息——真正的「回到源消息」
  靠气泡内的结果卡。
- **参考图上限 5 张**，客户端 canvas 压到长边 ≤1568px / ≤1MB（防 base64 token 爆炸）。
- **图片操作**：`⬇ 下载 / ↻ 重做 / 🖼 图生图`（图生图 = 把该图 URL 塞回附件列表，不重新上传）。

## 2. 本项目的技术约束（与参考项目不同，必须适配）

- 本项目是 **React 19 + TanStack Router/Query + Base UI + Tailwind**，参考项目是
  **无框架模板字符串**。**不能照搬 DOM 拼接**，必须用本项目组件与 hooks。
- `web/AGENTS.md` 强制：**先检索再复用**，用 `@/components/` 既有封装（Button/Dialog/
  ConfirmDialog/EmptyState…），新增替代实现必须记录能力缺口。
- i18n：所有文案走 `t()`，locale 文件**纯文本行插入，禁止 JSON 重序列化**
  （保护混淆键 `footer.newapi.projectAttributionSuffix`）。
- 测试：新增测试必须放模块专属 `__tests__/`，禁止与正式代码平铺。

## 3. 任务拆解与验收标准

| # | 子任务 | 交付物 | 验收标准 |
|---|---|---|---|
| T1 | 会话侧边栏（分组/新建/折叠/删除） | 新组件 + 持久化 | 真实浏览器 E2E：新建→出现在「今天」；折叠后状态在刷新后保持 |
| T2 | 模型选择按类别拆分 | 文本/图片/视频分离的选择器 | 切到图片模型时出现比例/分辨率；切到文本不显示 |
| T3 | 图片操作：下载 + 放大 | 图片卡片操作层 | E2E：对话中图片有下载按钮且可点击；点击可放大 |
| T4 | 参考图上传增强 | 上限提示 + 拖拽 + 张数显示 | E2E：拖拽上传 N 张；超出上限有明确提示 |
| T5 | 历史图片二次改图 | 「图生图」按钮 | E2E：点历史图片的图生图 → 该图进入附件 |
| T6 | Agent 提问选项卡片 | 选项 UI + 数字键 | E2E：agent 提问 → 出现选项 → 点选/输入自定义 |
| T7 | 画廊 | 会话图片网格 | E2E：生成的图片出现在画廊；可下载 |
| T8 | 比例/分辨率 | 「自动比例」等选项 | E2E：选择后请求参数正确 |
| T9 | **生成中骨架屏**（用户补充要求） | 图片/视频生成时的骨架占位 | E2E：提交后立即出现骨架、完成后被真实内容替换 |

## 4. 执行顺序

T1/T2 优先（结构性改动，其它功能挂在它们上面）→ T3/T4/T5（图片链路，用户最痛）
→ T6（需要后端配合，单独评估）→ T7（依赖 T3 的图片链接能力）→ T8。

## 5. 验证日志

（随执行追加）

### T1 / T2 完成（2026-10-13，v1.3.147）

**T1 会话侧边栏**：`components/sidebar/playground-sidebar.tsx`（新增，替换并删除了
旧的 `conversation-switcher.tsx` 下拉）。分组 今天/昨天/近7天/近30天/更早、
搜索、折叠（持久化）、删除确认。

**T2 模型分类**：`lib/models/model-kind.ts` + `api.ts` 接线。
根因是**数据缺失而非组件缺陷**——选择器本来就按 `category` 分组，
但 `getUserModels` 从不设置它。

#### 真实浏览器 E2E（Playwright + 真实网关 + 真实登录）
```
✅ 侧边栏可见            ✅ 时间分组标题: 今天
✅ 新建会话按钮可见      ✅ 搜索框可见
✅ 折叠按钮可见          ✅ 折叠后出现「展开」按钮
✅ 刷新后折叠状态保持    ✅ 模型分组标题: 文本
ℹ️ 展开宽度 256px，折叠后 48px
```
截图：`计划书/e2e-evidence/playground/{01-playground-loaded,02-after-new-chat,
03-collapsed,04-collapsed-persisted,05-model-selector-open,06-sidebar-zoom}.png`

#### 单测（34 项全绿）
- `conversation-grouping.test.ts` 22/22：跨自然日边界（23:59 vs 次日 00:01）、
  2/7/8/30/31 天边界、未来时间戳、NaN/Infinity、组内降序、空分组、
  输入不被修改、搜索空查询恢复全部、CJK 子串
- `use-sidebar-collapsed.test.ts` 6/6：默认展开、往返、键名锁定、
  **损坏值当展开**、读/写抛错均不崩
- `model-kind.test.ts` 15/15
- `api-models.test.ts` 3/3

#### ⚠️ 踩坑（下次直接用）
**前端是 `//go:embed web/dist` 嵌进 Go 二进制的**。改了前端必须
**先 `bun run build` 再 `go build`**，否则跑的是旧资源——我第一次 E2E 就踩了，
截图上还显示着已删除的「聊天」入口，白查一轮。

---

## T10 「Agent 内直接改图 + 批量生成」可行性评估（2026-10-13，读码实测）

用户要求：「二次改图可以 agent 内直接改啊，这样才够智能，就是说 AI 知道哪张图是不满意的、
要改哪张，然后要一次性做几张图？然后批量生成等等之类的」。

### 现状（逐项 grep 实测，非推断）

| 能力 | 现状 | 证据 |
|---|---|---|
| 后端多轮工具执行（agent loop） | ❌ **不存在** | `grep -rln "agent_loop\|AgentLoop\|toolLoop\|runAgent"` 零命中 |
| 后端 tool_calls 处理 | ◐ **仅透传** | 命中都在各 `relay/channel/*`（协议转换），无执行循环 |
| 前端 tool 循环 | ❌ 不存在 | `grep -rln "ToolCall" src/features/playground/ src/lib/` 零命中 |
| 游乐场文生图入口 | ❌ 不存在 | `grep -rn "images/generations" src/features/playground/` 零命中 |
| 参数面板 `n`（张数） | ❌ 无 | `playground-parameters.ts` 只有 size/temperature/top_p/max_tokens/seed 等 |

### 结论：这是一个**独立产品面**，不是补一个按钮

用户要的「AI 知道该改哪张图」= 需要 agent 能：
1. 在对话里**调用工具**（`edit_image(url, instruction)` / `generate_image(prompt, n)`）；
2. 拿到工具结果后**继续推理**（多轮）；
3. 前端把工具调用与结果渲染成可交互卡片。

三项都不存在。且这与指南 G5-3「动作级审批」是**同一个问题**（tool 调用就是动作），
必须一起设计，否则会做出两套互不相容的审批。

**因此本轮不做**，与 `计划书/MCP-Gateway-设计落档.md` §4.3 的判断一致。
但用户要的**可立刻兑现**的部分（不需要 agent loop）已在本轮落地：

- ✅ 图片可下载 / 放大（T3）
- ✅ 历史图片一键送回输入框（T5）——这就是「改图」的手动形态，已 E2E 验证
- ✅ 参考图上传（T4），含体积保护
- ⏳ **批量生成（n 张）**：不需要 agent loop，是纯参数能力 → **本轮可做**

### 本轮追加范围（T10，只做可真实闭环的部分）

1. **`n`（生成张数）参数**：图片模型下暴露，带上界（复用后端 `dto.MaxImageN`=128，
   但前端给更实用的档位），请求体正确携带。
2. **批量参考图**：一次传多张参考图（已支持 `multiple`，需验证上限与提示）。
3 **批量生成的结果卡**：多张图并排展示 + 逐张下载 + 逐张「用作参考图」。

**明确不做**（并说明理由）：agent 自主判断改哪张图 —— 需要后端 agent loop，
与 MCP/G5-3 一起立项。
