# 视觉识别体系 — 反「AI 通用美学」界面改造指南

> **基线**：`VERSION` = `v1.3.181`，提交 `83e67f270`（`git describe` = `v1.3.180-1-g83e67f270`）
> **工作区状态**：已跟踪修改 **16** 项 + 未跟踪 **77** 项（含本文档与 `计划书/README.md`）；
> 本文档所有事实以 **提交 `83e67f270` 的代码** 为准，不把未提交内容当既有事实。
> **文档性质**：施工蓝图（设计侧）。**本文档没有改动任何生产代码。**
> **拍板状态**：D1–D6 已于 2026-10-22 全部拍板（见 §11），**V0/V1 可开工**。
> **前置阅读**：`计划书/尼尔森十项可用性原则-界面设计规范与整改指南.md`（可用性层）、
> `计划书/下一步改进指南.md`（工程与版本层）、`web/AGENTS.md §3.10/§3.17`、`AGENTS.md`（前端规则）。

---

## 0. 先读这一段

### 0.1 本文档解决什么问题

Nielsen 指南解决的是「**能不能用好**」（四态、破坏性三级、错误文案三层、术语一致）。
本文档解决的是「**像不像一个有人做过设计的产品**」——视觉识别、排版、空间、质感、动效意图。
两者不重叠、不互相替代：一个页面可以完全符合 Nielsen 42 条却依然是一堵灰白卡片墙。

具体要解决的三件事：

1. **控制台视觉无识别度**：白底 + `rounded-xl` 卡墙 + 粉彩图标底 + 默认蓝主色，与任何 AI 生成的
   后台模板无法区分（证据见 §2.2）。
2. **令牌层不完整**：颜色/圆角/阴影有令牌，但**表面层级、规则线、数字排版、动效曲线、背景氛围**没有，
   导致每个 feature 自行发明观感（`bg-emerald-50` 87 处直写调色板即为后果）。
3. **落地页与产品页风格断裂**：落地页已是 editorial + motion + WebGL（`features/home`），
   进入控制台后视觉语言完全换了一套，用户从「有设计的产品」掉进「模板后台」。

### 0.2 与既有文档的边界（避免重复施工）

| 文档 | 负责层 | 本文档**不**碰 |
|---|---|---|
| `尼尔森十项可用性原则…md` | 可用性：四态、提交契约、破坏性三级、错误三层、术语、a11y | 不重列 Nielsen 条款，只引用编号 |
| `下一步改进指南.md` | 工程：版本口径、CI 门禁、部署、P0–P3 工程问题 | 不重排工程任务，只挂接版本号 |
| `ops/黄金范例.md` | 代码写法 | 不改代码风格约定 |
| **本文档** | **视觉：识别度、令牌、排版、空间、质感、动效意图** | 不新增业务功能、不换 UI 库、不改组件 API 语义 |

### 0.3 本轮实际运行过的命令（`已验证`）

```bash
cat VERSION                                   # v1.3.181
git describe --tags                           # v1.3.180-1-g83e67f270
git tag --sort=-creatordate | head -5         # v1.3.180 为最新 tag
sed -n '1,40p' CHANGELOG.md                   # 最新条目 [v1.3.180]，无 [v1.3.181]
git status --porcelain | wc -l                # 92
ls web/src/features | wc -l                   # 36
ls web/src/i18n/locales/                      # en/zh/zh-TW/fr/ru/ja/vi + *.common.json
rg -l "from '@/components/ui/card'" web/src --glob '*.tsx' | wc -l        # 42
rg -l "DataTable" web/src/features --glob '*.tsx' | wc -l                 # 82
rg -l "StatusBadge" web/src --glob '*.tsx' | wc -l                        # 78
rg -o "rounded-(lg|md|full|xl|sm|2xl)" web/src/features web/src/components | ...
#   rounded-lg 333 / rounded-md 288 / rounded-full 129 / rounded-xl 68 / rounded-sm 36 / rounded-2xl 35
rg -o "shadow-(none|sm|xs|md|card|lg|2xl|xl|raised|drawer)" ...
#   none 23 / sm 21 / xs 19 / md 15 / card 8 / lg 7 / 2xl 3 / xl 2 / raised 1 / drawer 1
rg -l "font-mono" web/src/features web/src/components | wc -l              # 169
rg -l "tabular-nums" web/src/features web/src/components | wc -l           # 101
rg -o "bg-emerald-50" web/src/features | wc -l                            # 44
rg -o "dark:bg-emerald-[0-9]+" web/src/features | wc -l                   # 14（暗色配对严重不足）
rg -n "font-family|@font-face|@import '@fontsource" web/src/styles/index.css
sed -n '1,40p' .github/workflows/ci.yml ; sed -n '88,118p' .github/workflows/ci.yml
```

> **版本口径事实（不修正，只记录）**：`VERSION` 已是 `v1.3.181`，最新 tag 仍是 `v1.3.180`，
> `CHANGELOG.md` 无 `[v1.3.181]` 条目 —— 三处口径不一致。本文档每批次交付时**必须**同步三处，
> 否则重犯 `下一步改进指南.md §3.3` 记录的老问题。

### 0.4 本文档不做什么

- 不引入新 UI 组件库（Base UI + `src/components/ui/` 已够用），不装 registry 模板。
- 不替换 Tailwind、不换图标库（Hugeicons 已是现状）、不动 VChart。
- 不删除 10 套主题预设（那是用户可见功能），只重新定义它们的**职责边界**。
- 不引入 CJK webfont（体积红线，见 §1.3）。
- 不触碰 `new-api` / `QuantumNous` 品牌标识（`AGENTS.md` 保护条款）。
- 不改任何用户可见文案语义（i18n 键不动，只动呈现）。

---

## 1. 设计命题（Design Before Code）

### 1.1 PURPOSE（目的）

| 问题 | 答案 |
|---|---|
| 这个界面解决什么问题 | 让开发者在一个地方看清「我的 token 花在哪、渠道是否健康、密钥是否安全」并完成配置 |
| 目标用户是谁 | 两类：**接入方开发者**（按天看用量与错误）与**站点运营者**（渠道、计费、审计） |
| 情感需求 | 「**可信、可读、可核对**」。不是「惊艳」，是「数字我敢信，配置我敢按」 |
| 反面情感 | 现在的粉彩图标 + 圆角卡片传达的是「演示环境」，与「管钱管密钥」的严肃度冲突 |

**结论**：控制台的正确美学不是「酷」，而是**仪器的克制**——仪表盘的读数是主体，界面必须退到读数之后。

### 1.2 TONE（调性：三候选 + 推荐）

| 候选 | 定义 | 优点 | 致命弱点 | 结论 |
|---|---|---|---|---|
| **A. 静默仪表（Quiet Instrument）** | 中性主色 + 单一信号色；细规则线分区；层叠表面代替阴影；等宽数字；极细网格底 | 与 10 套预设 + 暗色完全兼容；中文 UI 不依赖字体也能拉开差异；a11y 风险最低 | 需要很强的一致性纪律，否则退化成"另一种灰" | **推荐** |
| B. 粗野仪表盘（Brutalist Console） | 2px 硬边框、零圆角、硬投影、反白大写 micro-label | 记忆点极强 | 与现有 333 处 `rounded-lg`、65 个 `ui/` 组件冲突，等于重写；中文大写块排版坏；易读性下降 | 否决（可仅在营销页局部借用） |
| C. 暗色科技（Cyber） | 深底 + 青色发光 + 扫描线 | 与 AI 产品相似度最高、"一眼是 AI 产品" | **正是要规避的通用美学**；项目默认浅色；发光对比度难守 AA | 否决 |

**推荐 A，理由写入验收口径**：A 的差异化来自**结构**（非对称指标区、规则线分区、密度节奏）而不是
**装饰**（发光、渐变、字体噱头）——后者恰是 AI 生成界面的特征。

### 1.3 CONSTRAINTS（硬约束，全部来自现有代码与规则）

| 约束 | 来源 | 对设计的影响 |
|---|---|---|
| 10 套配色预设由用户自选，还有暗色 | `lib/theme-customization.ts:26-82`、`styles/theme-presets.css` | 视觉语言必须**令牌驱动**，不能写死任何具体色值；新令牌一律由 `--background/--foreground/--primary` 派生 |
| 中文是主语言，但项目**没有 CJK webfont** | `styles/index.css:22,28` 只引入 Public Sans / Lora | 中文字形由系统决定 → **排版差异化只能发生在拉丁/数字层**（等宽数字、micro-label），中文靠字重/字距/尺度 |
| 字体只能自托管（fontsource） | `package.json` 依赖形态、无 CDN `<link>` | 新增字体 = 新增 npm 依赖 + 体积；Lora（100KB 级）已是上限参考 |
| 组件复用强制 | `web/AGENTS.md §3.3`、`AGENTS.md` 前端规则 | 不许新造按钮/弹窗/表格；只能扩展 `Card`、`DataTable`、`StatusBadge` 等既有件 |
| i18n 强制，7 语言 | `web/AGENTS.md §3.1`、`locales/` | micro-label 不能靠 CSS `text-transform: uppercase` 表达语义（中文无效），要靠字距 + 字号 + 颜色 |
| WCAG 2.1 AA + axe 基线 0 违规（8 路由） | `CHANGELOG.md [v1.3.178]` | 任何"低对比灰字"方案直接违规；正文 ≥4.5:1，大字号 ≥3:1 |
| 触屏 ≥44px | `web/AGENTS.md §3.17` | 密度提升不能压穿触控目标 |
| 首屏性能 | 历史 `perf(web)` 提交（three.js 延后、KaTeX 移出首屏） | 背景质感不许用大图/大 canvas；只用 CSS 渐变或 inline SVG |
| CI 门禁 | `.github/workflows/ci.yml:100-118`：oxlint(changed) → typecheck → knip → vitest | 每批必须过这四关，视觉改动也不例外 |
| 生产计费/权限语义不可变 | 记忆库 `never-change-production-billing-directly.md` | 视觉改造不碰任何计费表达式与显示口径 |

### 1.4 DIFFERENTIATION（记忆点：用户离开后会记得什么）

三个**只属于本项目**的视觉签名（不是通用装饰）：

1. **「读数条」指标区**：控制台顶部一横条，指标之间用**细竖规则线**分隔、无卡片、无粉彩、数字用等宽字体
   右对齐、变化量用 ± 与箭头而非彩色胶囊。用户会记得"这个后台的数字像示波器"。
2. **网格底空态**：空数据时不是「一个灰色图标 + No data」，而是一块极细工程网格（2% 前景色）+ 一句
   可直接执行的动作 → 视觉上像工程图纸，功能上是 Nielsen 空态契约的强化。
3. **信号色纪律**：整站只有三种东西可以用强调色——**可操作、当前选中、异常**。指标卡图标底、装饰性
   色块、图表序列全部退回中性。用户会记得"这个系统不乱用颜色，所以颜色出现时一定有意义"。

---

## 2. 现状审计：现在的「AI 通用美学」具体在哪里（真实证据）

### 2.1 已具备的资产（**不要重造**）

| 资产 | 位置 | 说明 |
|---|---|---|
| 完整色彩令牌（oklch + 亮暗双套 + 语义 success/warning/info） | `styles/theme.css:106-219`（亮）/ `:221-302`（暗） | 含 `--success-text:145`、`--warning-text:149` 等专用文本令牌（a11y 已收口） |
| 语义高度令牌 6 档 | `styles/theme.css:211-218`（`--elevation-card/raised/hover/drawer/popover/overlay`） | **已有但几乎没人用**（见 §2.2-P2c） |
| 主题预设 10 套 + 字体轴 + 圆角轴 + 密度轴 + 内容宽度轴 | `lib/theme-customization.ts:26-82`、`theme-presets.css:624/681/698/731` | 用户可见功能，改造必须兼容 |
| 外观抽屉（预设/字体/圆角/密度/侧栏/布局/方向） | `components/config-drawer.tsx`（745 行，9 个 Section） | 新增视觉选项的落点 |
| 状态徽章统一件 | `components/status-badge.tsx`（78 文件在用） | 单一状态观感的既有基础 |
| 数据表体系 | `components/data-table/**`（82 文件在用，含 `README.md`） | 密集区改造的唯一合法入口 |
| 等宽 + 表格数字现状 | `font-mono` 169 文件、`tabular-nums` 101 文件 | 说明"数字要等宽"已是团队直觉，只是没成为规范 |
| 落地页动效体系 | `features/home/components/motion/`（reveal/magnetic/parallax/tilt）+ `motion/react` + `useReducedMotion` | 动效基础设施已存在，缺的是**标准** |
| 图表主题钩子 | `lib/use-chart-theme.ts` | 图表配色统一入口 |
| 玻璃/渐变工具类 | `styles/index.css:283+`（`glass-1..5`、`.bg-radial`） | 见 P2b：注释写着"matching Launch UI template" |

### 2.2 缺口清单（带证据）

#### P1（视觉识别度崩塌，必改）

**P1-a　卡片墙：42 个文件直接 `import Card`，控制台每屏 3–7 个同款 `rounded-xl` 容器**
证据：`rg -l "from '@/components/ui/card'" | wc -l` = 42；`components/ui/card.tsx:33` 固定
`rounded-xl py-4 ring-1`；生产截图 `web/e2e-local/prod-admin-_dashboard_models.png` 显示
「5 个等宽指标卡 + 2 个通栏卡」的典型 dashboard-by-numbers 构图。
后果：所有区块视觉权重相同 → 无层次 → 眼睛无落点。

**P1-b　粉彩图标底与直写调色板：绕过令牌系统，暗色/预设下观感崩坏**
证据：`bg-emerald-50` 44 处、`bg-amber-50` 43 处、`bg-sky-50` 9、`bg-violet-50` 8、`bg-rose-50` 5…；
而暗色配对 `dark:bg-emerald-*` 仅 14 处（配对比 44:14）。
例：`features/system-info/components/system-tasks-panel.tsx:59,63`、`features/setup/components/database-step.tsx:110,138`。
后果：切到 10 套预设中的任意一套，这些色块完全不跟随主题；暗色模式下部分色块对比度不可控。

**P1-c　均匀圆角：`--radius: 1rem` 单一基数，全站 333 个 `rounded-lg` + 288 个 `rounded-md` + 35 个 `rounded-2xl`**
证据：`styles/theme.css:107`、§0.3 计数。后果：所有容器/控件/标签圆角几乎一样 → "模板感"的主要来源之一。

**P1-d　字体轴本身是通用美学的例子**
证据：`config/fonts.ts` 提供 `inter / manrope / system`（三个最通用的脸），且**只被
`context/font-provider.tsx` 引用**；实际生效的字体轴是 `theme-customization.ts` 的 `default|sans|serif`，
sans 走 `--font-sans: 'Public Sans'`（`styles/theme.css:22`）。后果：字体选择没有观点，
且两套字体轴口径并行（**已拍板 D5：删除 `fonts.ts`**，见 V0）。

#### P2（体验层明显掉档）

**P2-a　阴影体系失序**：`shadow-none 23 / sm 21 / xs 19 / md 15 / card 8 / lg 7 / 2xl 3 / xl 2` 与语义令牌
（`shadow-raised 1 / shadow-drawer 1`）严重脱节 → 到底"抬起"是什么样没有共识。

**P2-b　玻璃工具类来自模板**
证据：`styles/index.css:283` 注释 `/* Glass morphism effects - matching Launch UI template */`，
`:304` 同样注释。这类"模板遗留"正是用户 prompt 里点名的「AI 通用美学」物证。

**P2-c　背景零氛围**：控制台 `features/**` 中 `bg-gradient/radial-gradient/noise` 计数几乎全在
`home`/`pricing`/`rankings`（营销页），控制台是纯 `bg-background` 平涂。

**P2-d　图表配色是"AI 默认五色"**：`styles/theme.css:158-164` `--chart-1..5` 为蓝/青/紫/品红/绿系
（`0.72 0.18 250`、`0.65 0.15 200`、`0.7 0.12 280`、`0.68 0.19 325`、`0.68 0.16 155`），与"单一信号色"纪律冲突。

**P2-e　指标数字无统一排版件**：`tabular-nums` 101 文件各自手写 `text-2xl font-semibold`，
单位大小、千分位、正负号、空值 `—` 的处理各不相同。

#### P3（长期）

- 图标尺寸无梯度规范（`size-4/5/6` 混用）。
- 空态/错误态的视觉语言靠各 feature 自拼（虽已用 `EmptyState`/`ErrorState` 组件，但无质感统一）。
- 深浅色下"表面层级"没有语义名（只有 `background/card/muted`），表达不了 3 层以上结构。

### 2.3 反例清单（不要照抄）

| 反例 | 为什么不要 |
|---|---|
| `rounded-2xl bg-emerald-500/10 p-4 text-emerald-600` 大色块提示 | 双重反模式：均匀大圆角 + 直写调色板（`features/setup/components/complete-step.tsx:144`） |
| 指标卡 5 连排、每卡一个粉彩圆角图标底 | 装饰性用色 + 等权重复，见生产截图 |
| `transition-all` 全局过渡 | 用户 prompt 明令禁止；且拖累 INP |
| 为"独特"引入 display 字体做中文标题 | 项目无 CJK webfont，实测会退回系统字体 → 白付出体积 |

### 2.4 判定口径（怎样算"通过"）

一个页面视觉验收通过 ⇔ 下面 6 条全为真：

1. 屏内**强调色只出现在**可操作元素 / 当前选中 / 异常状态（截图可人工判读，无装饰性色块）。
2. 屏内**容器层级 ≤ 2 层**，且层次由表面明度差（≥2% oklch 前景混合）或规则线表达，不靠阴影堆叠。
3. 所有数字右对齐、`tabular-nums`、单位小一号且 `text-muted-foreground`。
4. 圆角只出现 3 档：控件（`rounded-lg`）、容器（`rounded-xl`）、药丸（`rounded-full`）；屏内不出现 `rounded-2xl`。
5. 暗色 + 至少 2 套预设（`default`、`anthropic`）下观感成立且 axe 0 违规。
6. 320 / 768 / 1440 三档无溢出、无 44px 触控破位。

---

## 3. 美学方向 A：静默仪表（Quiet Instrument）

### 3.1 一句话定义

> **界面是纸，读数是墨。** 用极细规则线分区、用表面明度分层、用等宽数字承载数据，
> 强调色只作为信号出现；装饰一律让位于可核对性。

### 3.2 五支柱落在本项目上的具体做法

| 支柱 | 通用原则 | 本项目落地（受 §1.3 约束） |
|---|---|---|
| 排版 | 2–3 种字体、清晰尺度阶梯、强字重对比 | **拉丁/数字层**：数字与 ID/密钥用等宽（新增 `--font-numeric`）；中文层不换字体，靠 `font-weight`（450/600）、`letter-spacing`（中文 0，拉丁 micro-label `+0.06em`）、尺度对比（11 / 13 / 14 / 16 / 20 / 28） |
| 色彩 | 单一主导 + 一个锐利点缀 | 主导 = 中性（由 `--background/--foreground` 派生 3 层表面）；点缀 = `--primary`，**仅三语义**（可操作/当前/异常）。预设只改点缀色与色温，不改结构 |
| 动效 | 只在高价值时刻 | 4 个时刻：抽屉/弹层进入（200ms）、列表首屏 stagger（≤6 项、每项 40ms）、保存成功一次性勾描（320ms）、数字刷新淡入（120ms）。其余 hover 120ms 只动 color/opacity |
| 空间 | 不对称、大胆负空间、非常规节奏 | 指标区 `1.6fr 1fr` 非对称；组间 32px / 组内 12px（非均匀）；表格页首行留白 24px 而非等距 16px；详情抽屉用 400px 固定宽 + 主区自适应 |
| 背景细节 | 渐变/噪点/光晕 | 仅两处：① 空态用 2% 前景色 24px 工程网格（`repeating-linear-gradient`，零图片、零 JS）；② 认证页/首屏用 3% 径向渐变。控制台数据区保持净面（避免干扰读数） |

### 3.3 令牌层提案（CSS 变量，亮暗由 `--foreground` 派生自动成立）

```css
/* 追加到 styles/theme.css 的 :root 与 .dark 之外——派生令牌无需暗色重复定义 */
:root {
  /* 表面层级：用前景色混入实现，天然适配 10 套预设与暗色 */
  --surface-1: color-mix(in oklch, var(--foreground) 2%, var(--background));
  --surface-2: color-mix(in oklch, var(--foreground) 4%, var(--background));
  --surface-3: color-mix(in oklch, var(--foreground) 6.5%, var(--background));

  /* 规则线（分区用，比 --border 更细更弱） */
  --rule: color-mix(in oklch, var(--foreground) 9%, transparent);
  --rule-strong: color-mix(in oklch, var(--foreground) 16%, transparent);

  /* 空态工程网格 */
  --grid-line: color-mix(in oklch, var(--foreground) 5%, transparent);
  --grid-step: 24px;

  /* 信号色弱化版（hover/当前选中），不做装饰 */
  --accent-quiet: color-mix(in oklch, var(--primary) 9%, var(--background));
  --accent-quiet-strong: color-mix(in oklch, var(--primary) 16%, var(--background));

  /* 数字排版 */
  --font-numeric: 'JetBrains Mono Variable', ui-monospace, SFMono-Regular, Menlo,
    Consolas, monospace;
  --tracking-micro: 0.06em;

  /* 动效 */
  --motion-fast: 120ms;
  --motion-base: 200ms;
  --motion-slow: 320ms;
  --ease-out: cubic-bezier(0.16, 1, 0.3, 1);
}

@theme inline {
  /* 让工具类可用：bg-surface-1 / border-rule / font-numeric 等 */
  --color-surface-1: var(--surface-1);
  --color-surface-2: var(--surface-2);
  --color-surface-3: var(--surface-3);
  --color-rule: var(--rule);
  --color-rule-strong: var(--rule-strong);
  --font-numeric: var(--font-numeric);
}
```

**圆角阶梯收敛**（改一处，全站生效）：
`--radius` 默认 `1rem → 0.625rem`（`theme.css:107`）；约定控件 `rounded-lg`、容器 `rounded-xl`、药丸 `rounded-full`，
控制台禁用 `rounded-2xl`（35 处逐批清除）。`--radius` 的 7 档乘数阶梯（`theme.css:87-93`）保留不动，
外观抽屉的圆角轴（`theme-presets.css:681`）继续工作。

**字体**：新增 `@fontsource-variable/jetbrains-mono`（≈45–60KB woff2 子集，仅拉丁数字/符号），
只用于 `--font-numeric`；**不**进 `--font-body`，不参与 `font-sans` 轴 → 中文正文零影响。
体积由 `bun run build` 产物实测核对（预算：新增 ≤60KB gzip；超标则退回 `ui-monospace` 系统栈）。

### 3.4 禁令清单（可直接 grep 的规则）

| 编号 | 禁令 | 检查方式 |
|---|---|---|
| B1 | 控制台禁用 `rounded-2xl` | `rg "rounded-2xl" web/src/features` |
| B2 | 禁用直写调色板状态色（`bg-{emerald,amber,sky,violet,rose,orange,green,teal,indigo,fuchsia}-{50,100}`、`text-emerald-600` 等） | `rg "bg-(emerald|amber|sky|violet|rose|orange|green|teal|indigo|fuchsia)-(50|100)"` |
| B3 | 禁用 `transition-all` | `rg "transition-all"` |
| B4 | 指标区禁用 `shadow-*`（层次用表面） | 人工走查 |
| B5 | 同一屏强调色不得用于装饰（图标底、无意义徽章） | 人工走查 + 截图 |
| B6 | 新增文案必须走 `t()`，micro-label 不得依赖 `uppercase` | `bun run i18n:check` + 走查 |
| B7 | 禁止新增依赖 CDN 的字体/图片 | `rg "fonts.googleapis\|https://.*\.woff"` |

> B2 是**存量债**：本轮仅统计 `bg-{调色板}-{50,100,500/10,500/15}` 四类即 ≥120 处（不含 `text-*`/`border-*`），
> 按批次收敛，不要求一次清零；新代码零容忍。

### 3.5 与 11 套预设的兼容矩阵（改造后必须成立）

| 预设 | 亮色 | 暗色 | 备注 |
|---|---|---|---|
| `default`（蓝） | 必测 | 必测 | 主回归对象 |
| `anthropic`（暖奶油 + 陶土，`theme-presets.css:474-495`） | 必测 | 必测 | 检验 `color-mix` 派生在暖色背景上不脏 |
| `ink`（本轮新增，**结构差异套**，见 V13） | 必测 | 必测 | 圆角/密度/字体三轴联动，且三轴可被用户单独覆盖 |
| `simple-large`（高对比黑白，`:295`） | 必测 | 抽测 | 检验规则线/表面在无彩色下仍可辨 |
| `underground`/`rose-garden`/`lake-view`/`sunset-glow`/`forest-whisper`/`ocean-breeze`/`lavender-dream` | 抽测 | 抽测 | 只验「强调色三语义」不被装饰用法污染 |

---

## 4. 组件级契约

> 全部改动均为**兼容扩展**（新增可选 prop / 新增变体），不改既有调用行为。
> 依据 `web/AGENTS.md §3.3`：先检索、再复用、最后才新增。

### 4.1 表面与容器 —— `components/ui/card.tsx`

新增可选 `tone?: 'default' | 'panel' | 'plain' | 'inset'`（默认 `default`，**现有调用零变化**）：

| tone | 视觉 | 用途 |
|---|---|---|
| `default` | 现状（`bg-card` + `ring-1` + `rounded-xl`） | 保持兼容 |
| `panel` | `bg-surface-2` + `border-rule`，无 ring、无 shadow | 控制台主容器（替代现在的"卡片墙"） |
| `plain` | 无背景无边框，仅间距 | 分组容器（靠规则线分区） |
| `inset` | `bg-surface-1` + 内嵌 1px rule | 卡片内二级区（图表、表格容器） |

禁则：`shadow-*` 不得与 `panel`/`inset` 同用。

### 4.2 指标 —— 新建 `components/metric/`（唯一允许的新公共件）

`Metric` 组成（4 个插槽 + 2 个可选）：

```tsx
<Metric>
  <MetricLabel>总 TOKEN 数</MetricLabel>        {/* 11px / muted / 中文无字距 */}
  <MetricValue unit="M">128.4</MetricValue>     {/* font-numeric / 28px / tabular / 右对齐 */}
  <MetricDelta trend="up" value="+3.2%">较昨日</MetricDelta>  {/* 中性色 + 箭头，禁彩色胶囊 */}
  <MetricHint>近 24 小时</MetricHint>
</Metric>
```

硬规则：① 值一定 `tabular-nums`；② 单位小两号且 muted；③ 空值统一 `—`（`aria-label` 给"无数据"）；
④ delta 的正负**不得只用颜色**（Nielsen §3.7 一致性契约）。

### 4.3 密集数据区 —— `components/data-table/**`

- 表头改 micro-label（11px、muted、`tracking-micro`），**不加 uppercase**（中文无效且破坏 i18n）。
- 数字列右对齐 + `font-numeric`；金额列统一 2 位小数 + 千分位，用既有 `log-cost-display` 语义。
- 行高沿用密度轴（`theme-presets.css:698`），`pointer:coarse` 下 ≥44px 不变。
- 表格外框统一 `tone="inset"`；工具栏与分页用 `--rule` 分隔，不再套 Card。

### 4.4 状态与徽章 —— `components/status-badge.tsx`

- 收敛为"文字 + 小圆点"或"带底色胶囊"**二选一**（`web/AGENTS.md §3.17` 已有此约束），
  实现改为令牌色（`--success/--warning/--destructive` + `*-text` 令牌），清除调色板直写。
- 状态点必须有 `role`/`aria-label`（现状 `StatusDot` 已合规范，扩到全部状态位）。

### 4.5 图标与色块

- 图标尺寸三档：`size-4`（行内）/ `size-5`（按钮/表头）/ `size-6`（空态、卡片头）。
- **取消粉彩圆角底**：指标/区块图标改为无底色单色图标；确需强调时用 `--accent-quiet` 方形（`rounded-lg`）。

### 4.6 图表 —— `lib/use-chart-theme.ts`

- 序列色改为**单色顺序阶 + 1 个信号色**：`chart-1..5` 保留键名，值改为同色相不同明度
  （`color-mix(in oklch, var(--primary) X%, var(--background))` 逐步递进）。
- 网格线用 `--rule`，坐标轴文字 `--muted-foreground`（对比度已达标）。
- 图例默认关闭（≤3 序列时用直接标注）。

### 4.7 空/错/载三态视觉

| 态 | 现有件 | 视觉升级 |
|---|---|---|
| 空 | `EmptyState` + `emptyAction` | 网格底（`--grid-line` + `--grid-step`）+ 单色图标 + 一句动作；动作必须是可点按钮（Nielsen 契约不变） |
| 错 | `ErrorState` + `onRetry` | 顶部一条 `--rule-strong` 分隔 + 等宽 Request ID（可复制，已有 `CopyButton`）+ 重试按钮 |
| 载 | `Skeleton` / `LoadingState` | 骨架条用 `--surface-2`，去掉 shimmer 彩色；行数按真实列结构给 3/5/8 档 |

### 4.8 按钮与操作层级

- 每屏**至多 1 个** `default`（primary）按钮；次级用 `secondary`/`ghost`（`button.tsx:32-39` 已有变体，不新增）。
- 破坏性操作沿用 `destructive` + `ConfirmDialog` 三级契约（Nielsen §3.3），视觉上必须与普通按钮在**灰度下**
  也能区分（颜色 + 文案 + 图标至少两项）。

### 4.9 表单与抽屉

- 表单标签 13px、helper 12px muted；错误文字用 `--destructive` 且 `aria-invalid` 同步。
- 抽屉（`ConfigDrawer` 及各 Drawer）标题区用 `--rule` 分隔，宽度 400/480 两档；进入动画 `--motion-base` + `--ease-out`。

---

## 5. 动效规范

### 5.1 只保留四个高价值时刻

| 时刻 | 时长/曲线 | 说明 |
|---|---|---|
| 抽屉 / 弹层进入 | 200ms `--ease-out`，位移 8px + opacity | 已有 `tw-animate-css`，改参数即可 |
| 列表首屏 stagger | 每项 40ms，最多 6 项（>6 项不 stagger） | 只用于首屏一次，滚动加载不再触发 |
| 保存成功一次性勾描 | 320ms 描边 | 替代"只弹 toast"的弱反馈 |
| 数字刷新淡入 | 120ms opacity | 数据轮询更新时使用；不写"数字滚动"（可读性差、易误读） |

### 5.2 红线

- 禁 `transition-all`；只允许 `transition-colors` / `transition-opacity` / `transition-transform`。
- 全部动效必须被 `prefers-reduced-motion` 关闭或降级为即时（项目已有此范式：
  `features/home/components/sections/hero.tsx` 用 `useReducedMotion`，`styles/index.css` 有 8 处 media query）。
- 不动 `width/height/top/left/margin/padding`（`rules/web/coding-style.md`）。
- 落地页 WebGL / three.js 现状**不动**（已有性能收口：移动端不加载）。

---

## 6. 页面级改造清单

> 36 个 feature 目录不可能一次改完。按「用户每天看什么」排序，其余维持现状直到轮到。

| # | 页面/区域 | 现状问题 | 目标 | 批次 |
|---|---|---|---|---|
| 1 | 控制台概览 `features/dashboard` | 5 等权指标卡 + 粉彩图标底 + 通栏卡 | 读数条（1.6fr 1fr）+ 单一信号色 + 表面对比 | V1（试点） |
| 2 | `features/keys` | 卡墙 + 表格套卡 | `panel` 容器 + `inset` 表格 + micro-label 表头 | V2 |
| 3 | `features/models` / `features/pricing` | 模型卡片各自成风 | 统一 `panel` + 数字排版 + 计费徽章令牌化 | V2 |
| 4 | `features/usage-logs`（含 `log-cost-display`、`model-badge`） | 数字排版不统一、粉彩标签 | `font-numeric` + 令牌色徽章 | V2 |
| 5 | `features/channels` / `features/users` | 状态点多套观感 | `StatusBadge` 单一实现 + 行密度对齐 | V3 |
| 6 | `features/wallet` / `features/subscriptions` | 金额展示口径不一 | 金额排版契约 + 表面对比 | V3 |
| 7 | `features/playground` / `features/chat` | 输入区与消息区质感差异 | 输入区 `inset` + 消息气泡中性化（仅保留一档强调） | V3 |
| 8 | 外观抽屉 `components/config-drawer.tsx` | 9 个 Section 无视觉分组 | 规则线分组 + 预览区统一 | V4 |
| 9 | `features/auth/*`（登录/注册/找回） | 纯表单页 | 径向渐变底 + 单色卡 + 品牌锁 `σ` 级克制 | V4 |
| 10 | `features/status` / `features/errors` | 状态页信息密度低、错误页以状态码为主体 | 仪表化状态条 + 错误三层（Nielsen §3.4） | V4 |
| 11 | `features/home`（落地页） | 已有 editorial + motion，但 token 与内页不共享 | **只做 token 对齐**，不重设计 | V5 |
| 12 | 图表/空态/动效全站统一 | 见 §4.6/4.7/§5 | 统一 + 全量回归 | V5 |

---

## 7. 任务卡（V0–V13）

> **状态速览（2026-10-22）**：V0 ✅ / V1 ✅ / V2 ✅ / V3 ◐（dashboard 三个消费点已接，keys·wallet·status·channels 待接）/ V4–V13 ⏳
> 已完成项的证据：`CHANGELOG.md [v1.3.182]`、`test_ledger.md`（2026-10-22 段）、
> `计划书/e2e-evidence/v1.3.182/RESULTS.md`（10 张真实浏览器截图 + axe 0 违规）。

> 每张卡：目标 / 范围（含禁区）/ 交付物 / 验收 / 依赖。**未写验收命令的卡不允许开工。**

### V0　死配置清理：删除 `config/fonts.ts`（拍板 D5）
- **目标**：消除"字体轴两套并行口径"，`lib/theme-customization.ts` 的 `default|sans|serif` 成为唯一真相。
- **范围**：删 `web/src/config/fonts.ts`；移除 `context/font-provider.tsx` 的引用（若该引用只是重新导出则一并清理）。
- **禁区**：不改字体轴的解析逻辑（`resolveThemeFont`/`PRESET_DEFAULT_FONT`）、不动 `theme-presets.css` 的字体块。
- **交付物**：删除 + 引用清理 + 一句 CHANGELOG 说明（"移除未生效的字体清单配置"）。
- **验收**：`bun run typecheck` / `bun run lint` PASS；`bun run knip` 无新增死代码；外观抽屉字体轴仍可切换（手动走查 + 截图）。
- **依赖**：无（可与 V1 同批）。

### V1　令牌层落地（无视觉变化的一次性基础）
- **目标**：`--surface-1..3`、`--rule*`、`--grid-*`、`--accent-quiet*`、`--font-numeric`、`--motion-*`、`--ease-out` 全部进入 `theme.css` 并在 `@theme inline` 暴露工具类；`--radius` 1rem → 0.625rem（拍板 D2）；新增 `@fontsource-variable/jetbrains-mono` 依赖（拍板 D3）。
- **范围**：`web/src/styles/theme.css`。**禁区**：不改 `theme-presets.css` 的 10 套预设值、不动任何组件。
- **交付物**：令牌 + 一段"令牌字典"注释（每个令牌一行用途）。
- **验收**：`bun run typecheck` / `bun run build` PASS；**build 前后产物体积差值记录**（字体新增 ≤60KB gzip，超预算即退回系统栈并在文档登记）；圆角前后对比截图（3 页 × 亮暗）；暗色 3 页抽检无对比度回退。
- **依赖**：无（可与 V0 同批）。

### V2　`Card.tone` 兼容扩展 + 试点页（dashboard 概览）
- **目标**：`Card` 增 `tone`；dashboard 改为「读数条 + 两段式布局」，清掉粉彩图标底与等权卡片。
- **范围**：`components/ui/card.tsx`、`features/dashboard/**`。
- **禁区**：不改 dashboard 的任何查询/数据口径；不动 `use-chart-theme.ts`（留到 V9）。
- **交付物**：新构图 + 令牌化容器 + 一致性单测（`tone` 类名映射）+ 三档截图。
- **验收**：`bun run test -- src/components src/features/dashboard`；截图 320/768/1440 × default/anthropic × light/dark；axe 该页 0 违规。
- **依赖**：V1。

### V3　`Metric` 组件 + 数字排版契约
- **目标**：新建 `components/metric/`，替换 6 个高频指标位（dashboard、keys、wallet、usage-logs 汇总、channels 健康、status 页）。
- **范围**：新增组件目录 + 上述 feature 的展示组件。**禁区**：不改任何 i18n 键值语义（可复用既有键）。
- **验收**：组件单测（等宽/对齐/空值 `—`/delta 无色彩依赖）；`bun run typecheck`；截图。
- **依赖**：V1。

### V4　`DataTable` 视觉统一（表头 micro-label、数字列右对齐、外框 `inset`）
- **目标**：在 `components/data-table/**` 内做一次集中改造，覆盖 82 个使用方而不逐个改。
- **范围**：`components/data-table/**` + `components/ui/table.tsx`。
- **禁区**：不改排序/筛选/分页/虚拟滚动行为；不动 `README.md` 里承诺的公开导出。
- **验收**：`bun run test -- src/components/data-table`；keys/usage-logs/models 三页截图；行高在 `pointer:coarse` 下 ≥44px。
- **依赖**：V1。

### V5　状态色令牌化收口（清 B2 存量前 3 批）
- **目标**：`StatusBadge`/`Alert`/`Badge` 三件套改令牌色；按 P1 文件清单分批清除 `bg-*-50`（先 44 处 emerald、43 处 amber 的热点文件）。
- **范围**：`components/status-badge.tsx`、`components/ui/{badge,alert}.tsx`、目标 feature。
- **禁区**：不改语义（enabled/disabled、成功/警告）；不改文案。
- **验收**：`rg "bg-(emerald|amber|sky|violet|rose)-(50|100)"` 计数下降并可列举；暗色 + 2 预设截图；axe 0 违规。
- **依赖**：V1。

### V6　图表主题（单色阶 + 信号色）
- **目标**：`use-chart-theme.ts` 输出单色顺序阶；图例规则统一。
- **范围**：`lib/use-chart-theme.ts` + 图表调用点（dashboard/ops-panel/rankings 的消费方）。
- **禁区**：不改数据结构与 VChart spec 的字段契约。
- **验收**：图表页截图（亮/暗）；`bun run test` 相关文件；`bun run typecheck`。
- **依赖**：V2。

### V7　空/错/载三态视觉
- **目标**：`EmptyState` 网格底 + 单色图标；`ErrorState` 规则线 + 等宽 Request ID + 重试；`Skeleton` 去彩。
- **范围**：`components/{empty-state,loading-state,error-state}`、`components/ui/skeleton.tsx`。
- **禁区**：不改 `emptyAction` 契约（Nielsen 强制动作）；不改错误文案三层结构。
- **验收**：三态各 2 页截图；单测（动作存在、Request ID 可复制）。
- **依赖**：V1。

### V8　动效规范落地（4 时刻 + 红线）
- **目标**：全局动效令牌化；替换 `transition-all`；抽屉/弹层/stagger/勾描四时刻统一；`prefers-reduced-motion` 全覆盖。
- **范围**：`styles/index.css`、`components/ui/{drawer,dialog,sheet}.tsx`、`components/animate-in-view.tsx`。
- **禁区**：落地页 `motion/` 与 WebGL 不动。
- **验收**：`rg "transition-all"` = 0（或在豁免清单内）；reduced-motion 下列表无动画（单测 + 截图）；INP 抽测不劣化。
- **依赖**：V1。

### V9　控制台次要页（wallet / subscriptions / channels / users）
- **验收**：每页 3 档截图 + axe 0 违规 + 相关 vitest。
- **依赖**：V2–V5。

### V10　playground / chat 输入区
- **验收**：发送链路 E2E 手工走查（现有 `web/e2e-local/pg-*.cjs` 脚本可复用）+ 截图。
- **依赖**：V4。

### V11　auth / status / errors
- **验收**：`features/auth` 单测、`web/e2e-local/g7-status-page.cjs` 跑通、axe 0 违规。
- **依赖**：V7。

### V12　全站回归 + 视觉基线固化
- **目标**：把「截图矩阵」固化为可重跑脚本（`web/e2e-local/visual-baseline.cjs`），作为后续改动门禁。
- **范围**：新增脚本 + `计划书/e2e-evidence/visual-baseline/` 证据目录。**禁区**：不接入 CI 阻塞（先手动跑，稳定后再议）。
- **验收**：脚本一次运行产出 12 页 × 3 断点 × 2 预设 × 2 主题截图 + `results.json`；axe 8 路由 0 违规保持。
- **依赖**：全部。

### V13　`ink` 预设：结构差异套（拍板 D4，**执行顺序在 V12 之前**）
- **目标**：新增第 11 套预设 `ink`（石墨中性 + 单一信号色），且**结构层面**与现有预设不同：
  圆角阶梯更紧（容器 `rounded-lg` 级）、密度更紧凑（沿用密度轴 `sm` 基线）、字体轴默认 `sans` 但
  数字层强制 `--font-numeric`。三者必须能在外观抽屉里被用户单独覆盖（"跟随预设"语义）。
- **范围**：`lib/theme-customization.ts`（注册 `ink` + `PRESET_DEFAULT_FONT`/半径提示）、
  `styles/theme-presets.css`（`[data-theme-preset='ink']` 块）、`components/config-drawer.tsx`（若需"跟随预设"选项）。
- **禁区**：不改 8 套旧预设的值；不改 `THEME_RADIUS_VALUES`/`THEME_SCALE_VALUES` 的类型契约（只加解析提示）。
- **交付物**：预设注册 + CSS 块 + 抽屉内可选 + 回归单测（预设解析、`resolveThemeFont` 覆盖行为）。
- **验收**：`ink` × 亮/暗 × 3 断点截图；`default`/`anthropic` 观感零变化（对比截图）；axe 0 违规；抽屉内"圆角/密度/字体"三项在 ink 下可单独覆盖（单测 + 手动走查）。
- **依赖**：V1–V8（令牌与组件契约就位后才有"结构"可组合）。

---

## 8. 版本路线（每步必须有可交付价值）

| 版本 | 内容 | 可交付价值 |
|---|---|---|
| v1.3.182 | V0 + V1 + V2 | 令牌层 + 圆角收敛 + 等宽数字 + 概览页新构图（用户第一屏可见变化） |
| v1.3.183 | V3 + V4 | 数字与表格统一（最高频操作界面） |
| v1.3.184 | V5 + V6 | 颜色纪律 + 图表统一（消除暗色/预设崩坏） |
| v1.3.185 | V7 + V8 | 三态质感 + 动效标准（可感知的"精致感"） |
| v1.3.186–187 | V9–V11 | 次要页覆盖 |
| v1.3.188 | V13 | `ink` 结构差异套（给用户一个"有观点"的默认） |
| v1.3.189 | V12 | 视觉回归基线（覆盖 default/anthropic/ink 三套预设） |

**纪律**：每个版本同步 `VERSION` / tag / `CHANGELOG` 三处口径（当前 `v1.3.181` 已有一处缺失）。

---

## 9. 验收矩阵

### 9.1 自动化（能跑就必须跑）

| 检查 | 命令 | 通过标准 |
|---|---|---|
| 类型 | `cd web && bun run typecheck` | 0 error |
| Lint | `cd web && bun run lint` | 所涉文件 0 error |
| 单测 | `cd web && bun run test` | 全绿（既有噪声见 `计划书/audit-ledger.md`） |
| 死代码 | `cd web && node scripts/knip-gate.mjs` | 无新增死代码 |
| i18n | `cd web && bun run i18n:check` | 无缺失键 |
| 术语 | `cd web && bun run i18n:terminology` | quota 统一"额度" |
| 构建 | `cd web && bun run build` | 成功；记录产物体积变化 |
| a11y | axe 脚本（现有 `web/e2e-local/*.cjs` 模式，`axe-core` 已在 devDeps） | 8 路由 0 critical/serious（基线见 `CHANGELOG [v1.3.178]`） |
| 禁令扫描 | §3.4 的 `rg` 命令 | 新代码 0 命中 |

### 9.2 视觉证据（每批必存）

`计划书/e2e-evidence/visual-<版本>/`：
- 12 页 × 3 断点（320/768/1440）× {default, anthropic}（V13 起加 `ink`）× {light, dark} 的代表组合（每批只需覆盖本批改动页 + 概览回归页）。
- 命名：`<page>-<breakpoint>-<preset>-<theme>.png`；`results.json` 记录 URL、时间、哈希。

### 9.3 人工走查（每批 15 分钟，对照 §2.4 六条）

1. 概览页：强调色是否只出现在三语义？→ 截图判读。
2. 表格页：数字列是否右对齐 + 等宽？单位是否 muted？→ 放大截图。
3. 暗色切换：`bg-emerald-50` 类色块是否已消失/令牌化？→ 扫描 + 肉眼。
4. 320px：无横向滚动、44px 触控目标不破位。
5. 键盘：Tab 顺序 + 焦点环可见（`outline-ring/50` 已有基线）。
6. `prefers-reduced-motion: reduce`：列表/抽屉无动画（DevTools 渲染面板切换）。

---

## 10. 风险与回滚

### 10.1 风险登记

| 编号 | 风险 | 影响 | 缓解 |
|---|---|---|---|
| R1 | `--radius` 1rem→0.625rem 影响 65 个 `ui/` 组件与所有页面 | 观感全站变化，可能被业主判为"变丑" | **单独成批（V1）且先出截图给业主确认**；一行改回即可回滚 |
| R2 | 10 套预设 + 暗色下 `color-mix` 派生表面在暖底发脏 | anthropic 预设观感变差 | 派生比例只用 2/4/6.5%；矩阵 §3.5 必测；必要时为暖底预设加 `--surface-*-lift` 覆写 |
| R3 | `--font-numeric` 增加 45–60KB | 首屏体积 | 只用于数字（不回退到 `font-family: inherit` 的正文）；超预算退回系统等宽栈 |
| R4 | 清理 `bg-*-50` 触碰 100+ 文件 | 回归面大 | 按热点文件分批（V5 三次提交），每批限 ≤15 文件 |
| R5 | 视觉改动破坏既有 a11y 0 违规基线 | 门禁回退 | 每批 axe 必跑；新令牌先验证对比度（正文 ≥4.5:1） |
| R6 | 与业主正在进行的 N 系列（N1–N16）撞车 | 冲突/重复 | 本文档只改`视觉表现`，N 系列改`交互契约`；同文件冲突时以 Nielsen 指南为准 |
| R7 | 工作区脏文件混入视觉批次 | 提交污染 | **已处置（拍板 Q7）**：V1 前由业主提交 N15 成果；我提供分类清单，不自动 commit、不 `git clean`。V1 只动 `styles/theme.css` 与 `@theme inline`，与在途文件无重叠 |
| R8 | `ink` 做成结构差异套 ⇒ 预设体系进入"双轨"（同色不同结构） | 维护成本与测试矩阵翻倍；外观抽屉的圆角/密度/字体三轴需要"跟随预设"语义 | 三轴必须可被用户单独覆盖（不允许预设硬锁）；矩阵只把 `ink` 列为必测，其余 8 套仍抽测；若 V13 后维护痛感明显，退货为"仅色相差异"并登记决策 |

### 10.2 回滚策略

- **令牌层（V1）可整体回滚**：所有视觉派生都来自 §3.3 的 13 个新变量 + `--radius` 一行；
  回退即恢复旧观感，组件层的 `tone`/`Metric` 因默认值兼容而不报错。
- 每批只动一个版本号，tag 前保留上一版镜像路径（部署纪律见记忆库
  `freeapi-production-deploy.md`）：**本文档不触发任何生产部署**。

---

## 11. 拍板结论（2026-10-22 已定，V1 可开工）

| # | 决策项 | **结论** | 对施工的影响 / 落点 |
|---|---|---|---|
| D1 | 美学方向 | **A 静默仪表（Quiet Instrument）** | §3 全章生效；B/C 方案不再保留为备选 |
| D2 | 圆角基数 | **`--radius` 1rem → 0.625rem** | V1 实施；单独成批并先出对比截图（风险 R1） |
| D3 | 数字字体 | **引入 `@fontsource-variable/jetbrains-mono`** | V1 加依赖 + `--font-numeric`；只用于数字/ID/密钥，不进 `--font-body`；体积超预算则退回系统栈并记录 |
| D4 | ink 预设 | **新增 ink，且做成「结构差异套」**（不只换色：圆角阶梯 + 密度 + 字体轴联动） | 新增任务卡 **V13**；`ink` 独立版本交付；风险 R8（双轨维护、矩阵翻倍）需接受 |
| D5 | 死配置 | **删除 `web/src/config/fonts.ts`** | 新增任务卡 **V0**；`theme-customization.ts` 字体轴成为唯一真相 |
| D6 | 改造范围 | **控制台全面改造 + 落地页只做 token 对齐**（落地页不重设计、不动 WebGL/motion） | §6 清单第 11 行按"只对齐令牌"执行；落地页不得出现结构改动 |

**由此新增/调整的施工要求：**

1. **ink 是结构差异套** ⇒ 视觉回归矩阵从「2 套预设」升为「3 套预设」（`default` / `anthropic` / `ink`），
   且 `ink` 必须验证**圆角轴、密度轴、字体轴三者联动**不互相打架（外观抽屉 `config-drawer.tsx` 的
   Radius/Scale/Font 三个 Section 需支持"跟随预设"语义，而不是硬覆盖）。
2. **删 `fonts.ts` 属"顺手清理"** ⇒ 只允许删文件 + 移除 `context/font-provider.tsx` 的引用，
   不得顺带重构字体轴的其它逻辑（`AGENTS.md` 前端规则：不做未要求的改造）。
3. **落地页 token 对齐**只允许改：表面/规则线/数字排版/圆角三档的**取值来源**；
   禁止改 `features/home` 的布局、动效参数、WebGL 场景与 SEO 注入。

**仍未定（下一轮再问，不阻塞 V1）：**

| # | 事项 | 为什么可以延后 |
|---|---|---|
| E1 | `--chart-1..5` 单色阶的具体明度步进值 | V6 才动，届时可直接出 3 版对比截图定 |
| E2 | 是否给 `ink` 之外的 8 套旧预设也加"结构差异" | 先看 ink 的用户反馈，避免一次性把预设体系复杂化 |
| E3 | 视觉基线脚本是否接入 CI 阻塞 | V12 跑稳后再议（当前会影响 PR 门禁节奏） |

---

## 12. 未验证项（本文档不假装验证过）

1. **未运行任何前端命令**：本轮全部为只读审计（`rg`/`sed`/`ls`/`git`），**没有**执行 `bun run typecheck|test|build`，
   因此"改造后能否 typecheck 通过"属 `待验证`。
2. **未在浏览器中查看暗色 + 10 预设全矩阵**：`color-mix` 派生表面在 `anthropic`（暖奶油底）上的观感属 `合理推断`，
   必须 V1 实测。
3. **`--radius` 改动的完整影响面未穷举**：65 个 `ui/` 组件中哪些显式写死 `rounded-xl/2xl` 未逐一核对（`待验证`）。
4. **`JetBrains Mono` 体积未实测**：45–60KB 为 fontsource 常见值，需 `bun run build` 实测。
5. **axe 基线 0 违规**引用自 `CHANGELOG [v1.3.178]` 的记载，本轮**未重跑**。
6. **生产截图的生产版本号未知**：`web/e2e-local/prod-*.png` 未标注对应线上版本（`待验证`），
   仅作为"现状观感"证据使用，不作为线上升级证明。
7. **INP/LCP 影响未测**：动效与背景改造后需 `web/e2e-local/fe-vitals.cjs` 复测（现状数据见 `fe-vitals-*.json`）。
8. **`ink` 结构差异套的具体取值未定**：圆角紧到哪一档、密度用 `sm` 还是新档、字体轴默认值，
   需在 V13 出 2–3 版对比图后定稿（`待验证`，不阻塞 V1–V8）。
9. **"落地页只做 token 对齐"的边界未逐文件核对**：`features/home` 中哪些 class 属于"结构"、哪些属于"取值来源"
   尚未逐项分类（`待验证`，V5 之前必须产出该清单）。

---

## 13. 变更记录

| 日期 | 变更 | 依据 |
|---|---|---|
| 2026-10-22 | 初版：基线 v1.3.181 / 提交 `83e67f270`；三方向取舍；令牌提案；V1–V12 任务卡；6 项待拍板 | 本轮只读审计命令见 §0.3 |
| 2026-10-22 | **D1–D6 全部拍板并入册**：方向 A、圆角 0.625rem、引入 JetBrains Mono、`ink` 结构差异套、删除 `fonts.ts`、范围=控制台+落地页 token 对齐；新增 V0/V13 任务卡、R8 风险、§3.5 扩为 11 套预设矩阵、§8 拆出 v1.3.188/189 | 业主拍板（AskUserQuestion Q1–Q7） |
| 2026-10-22 | **v1.3.182 交付（V0+V1+V2+V3◐）**：令牌层落地、`--radius` 收敛、`font-numeric` 等宽数字、`Card.tone`、`Metric` 组件、dashboard 试点（rounded-2xl 归零）；本地真实浏览器 E2E 10 图 + axe 0 违规 | `CHANGELOG.md [v1.3.182]`、`test_ledger.md`、`计划书/e2e-evidence/v1.3.182/` |
