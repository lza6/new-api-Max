# 前端包体积基线（Bundle Size Baseline）

> 生成：2026-09-24 · `cd web && bun run build` 实测（rsbuild 2 生产构建）。
> 用途：记录当前基线 + 已落地优化 + 后续瘦身建议，避免重复盲跑。

## 基线（2026-09-24 实测）

| 指标 | 值 |
|---|---|
| 总 JS | 59,352 kB（gzip 17,115 kB） |
| 入口 index.js | 4,378 kB（gzip 1,245 kB） |
| 最大 async chunk | 9926: 6,827 kB（gzip 2,751 kB） |
| CSS | 427 kB（gzip 62 kB） |

Top 10 chunk（原始/gzip）：
1. 9926: 6,827 / 2,751 kB
2. 6685: 5,468 / 1,030 kB
3. 9197: 5,372 / 1,204 kB
4. 3898: 5,088 / 1,968 kB
5. index: 4,378 / 1,245 kB
6. 8496: 3,164 / 374 kB
7. vendor-charts: 2,307 / 618 kB
8. 5143: 2,148 / 478 kB
9. 240: 2,120 / 561 kB
10. 3239: 1,962 / 418 kB

## 已落地优化（侦察确认，非本次新增）
- 路由级 `React.lazy`：channels/dashboard/web-protection/users/usage-logs/system-settings 等
  全部懒加载（web/src/routes/_authenticated/*）。
- TanStack Router `autoCodeSplitting: isProd`（生产自动路由拆包）。
- vendor cacheGroups：vendor-charts（@visactor/recharts）、vendor-shiki（shiki）、
  vendor-editor（@codemirror）、vendor-icon-libs（@hugeicons/lucide）、vendor-react、
  vendor-ui-primitives（@base-ui/@radix-ui）、vendor-tanstack。
- `removeConsole: ['log']`（生产去 console.log）。

## 缺口与后续建议（P2，未在本批执行）
1. **index.js 4.38MB 同步入口**：路由层（routeTree.gen 静态导入全部 route 文件）与共享
   provider/工具库仍被打进入口。→ 用 rsbuild stats（`bunx rsbuild build --stats`）精确定位
   index chunk 内 Top 依赖再决定拆法（避免盲目拆分引入回归）。
2. 9926/6685/9197 等超大 async chunk：单个 5-7MB 说明某些 feature 页内仍有重依赖
   （图表/编辑器/表格虚拟化）。→ 页内二次懒加载 + 探索按需 import 子模块。
3. 首屏 JS 预算：当前 index+gzip ≈ 1.3MB，建议目标 < 800KB（后续批次）。

## 防重复跑
- 已记录本次 build 基线；下次改动 rsbuild.config.ts / 路由树 / 重依赖引入后再跑 build 对比。
- 本报告为唯一基线源，勿重复盲跑全量 build 仅为了「看数字」。
## 记录 0002 · T9-E2 入口瘦身结论（2026-09-24）
- **改造**: devtools 改为 DEV-only 动态 import（`__root.tsx`）——生产彻底排除 @tanstack/*-devtools，
  工程正确性收益；入口体积实测不变（4377.6 kB，说明 devtools 本就被 Rspack DCE 处理或异步分包）。
- **入口构成分析（dist 反查）**: index.js 仅含 katex/marked 轻库；shiki/recharts/visactor/codemirror/mermaid
  全部已异步分包（cacheGroups）。4.38MB 大头是业务代码（31 feature 路由树 + 共享组件层）。
- **结论**: 入口已是最优结构（重依赖全异步 + 路由级 autoCodeSplitting）；进一步瘦身需拆共享层，
  收益低风险高，**不推荐**。首屏 JS ≈ 1.25MB gzip 为合理基线。
- **防重复跑**: 不再重复全量 build 仅看 index 体积；改动入口/共享层后再对比。

## 记录 0003 · T9-E2/E3 最终核验（2026-09-24 第二轮）
- **E2 入口复测**: `bun run build` 实测 index.js **4379.4 kB（gzip 1246.2 kB）**，与记录 0002
  （4377.6 kB）一致 → 入口结构稳定，重依赖全异步分包 + 路由级 autoCodeSplitting 已是最优，
  **维持"不推荐进一步瘦身"结论**（拆共享层收益低风险高）。
- **E3 knip 死代码**: 收紧 `knip.config.ts`（移除过时的 tailwindcss/tw-animate-css
  ignoreDependencies，二者已被 rsbuild 插件追踪为使用）；配置提示 3→1，仅剩
  routeTree.gen.ts（TanStack Router 生成文件，保留 ignore 正确）。
  **无新增死代码**：批量定价组件（BulkPricingDialog）被 data-table-bulk-actions 引用，
  未被 knip 标记；68 个历史未使用文件/类型属存量，不清理（风险高、非本批范围）。
- **测试 mock 类型修复**: bulk-pricing-dialog.test.tsx 的 `options: {}` 缺 PricingOptions
  全字段导致 tsgo 报错，补齐 10 个 key → typecheck 绿 + vitest 3/3 绿。
- **结论**: T9-E2/E3 **闭环**。首屏 JS ≈ 1.25MB gzip 为合理基线；改动入口/共享层后再对比。