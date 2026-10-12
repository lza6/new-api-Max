/**
 * T9-4 性能预算回归测试
 *
 * 读取生产构建产物（web/dist/static/js）验证体积预算，防止依赖回归导致
 * 入口/总包体积爆炸。CI 顺序：bun run build → bunx vitest run（本文件）。
 * 若 dist 不存在（纯单测阶段），测试跳过而非失败。
 *
 * §4.2.5 扩展：递归扫描 async/ 等子目录（路由级异步 chunk 才是真正的
 * 页面体积大头），并复核 code-splitting 生效——新页面必须拆成独立
 * 异步 chunk，而不是全部打进 index。
 */
import { readdirSync, readFileSync, statSync, existsSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const distJsDir = path.resolve(process.cwd(), 'dist/static/js')

/** 递归收集 dist/static/js 下所有 .js 文件（含 async/ 等子目录），name 为相对该目录的路径。 */
function jsFiles(): { name: string; sizeBytes: number }[] {
  if (!existsSync(distJsDir)) return []
  const out: { name: string; sizeBytes: number }[] = []
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        walk(full)
      } else if (entry.isFile() && entry.name.endsWith('.js')) {
        out.push({ name: path.relative(distJsDir, full), sizeBytes: statSync(full).size })
      }
    }
  }
  walk(distJsDir)
  return out
}

function gzipEstimate(sizeBytes: number): number {
  // gzip 压缩比经验值：业务 JS 约 0.25-0.45；用 0.45 上限保守估算，
  // 避免测试因实际压缩比波动 flaky（真实阈值见预算常量）。
  return sizeBytes * 0.45
}

describe('bundle budget (production build)', () => {
  it('total JS stays under 24 MB raw / 11 MB gzip-estimated', () => {
    const files = jsFiles()
    if (files.length === 0) {
      return // dist 未构建（CI 单测阶段），跳过
    }
    const totalRaw = files.reduce((s, f) => s + f.sizeBytes, 0)
    // 2026-10-12 实测：react-icons 曾通过 `react-icon-by-name.tsx` 里的 31 个
    // `import('react-icons/<pack>')` 动态整包加载把 gi/pi/si/tb/md/fa 等全部打进
    // async chunk，`dist/static/js` 总 raw 达 59.35 MB。改为固定图标表 + 单次
    // 懒加载（`components/payment-icons.ts`）后降到 20.88 MB（−64.8%）。
    // 指南 G8 的目标是 ≤43 MB（59.35→降 ≥30%）；实测 20.88 MB 已明显低于该目标。
    // 阈值取 24 MB（= 实测 +15% 缓冲）：远低于旧阈值 70 MB（旧值高于实测，形同虚设），
    // 也低于旧实测 59.35 MB 与指南的 43 MB；而最小图标整包 gi 就有 6.5 MB，
    // pi 5.1 / si 4.9 / tb 3.0 / md 1.9 / fa 2.9，重新引入任一个都会立刻顶破。
    // 注意：**阈值必须高于当前实测值**，否则测试恒失败；24 MB 是"贴近实测 + 留少量
    // 正常功能增长空间"的取法，不是"低于实测值"（那样无法通过）。
    // 另：本用例守的是 dist/镜像体积，与首屏无关——首屏由下方 sync-chain 用例单独守
    //（那是用户真正等待的字节，实测 721 KB gzip）。
    expect(totalRaw).toBeLessThan(24 * 1024 * 1024)
    // gzipEstimate 用 0.45 保守上界。实测 raw 20.88 MB → 估算 9.40 MB；上限 11 MB。
    expect(gzipEstimate(totalRaw)).toBeLessThan(11 * 1024 * 1024)
  })

  it('index entry chunk stays under 1.2 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    const index = files.find((f) => f.name.startsWith('index.'))
    expect(index).toBeTruthy()
    // no-non-null-assertion 禁止 `!`：前置 toBeTruthy 已保证存在，此处用可选链。
    // 2026-10-12 实测 index.js ≈ 0.88 MB raw（react-icons 整包治理前后基本持平，
    // 因为图标数据本就在 async chunk）。1.2 MB 上限留 ~36% 缓冲，可抓住真正回归，
    // 又不因正常页面增长误报（旧值 1.5 MB 余量偏松）。
    expect(index?.sizeBytes ?? 0).toBeLessThan(1.2 * 1024 * 1024)
  })

  // 首屏 = index.html 里同步加载的 JS/CSS 链（不是「index chunk」）。
  // 这是用户真正等待的字节；旧测试只看 index chunk，漏掉了整条同步链。
  it('first-paint sync chain (from dist/index.html) stays under 740 KB gzip', () => {
    const htmlPath = path.resolve(process.cwd(), 'dist/index.html')
    if (!existsSync(htmlPath)) return // dist 未构建，跳过

    const html = readFileSync(htmlPath, 'utf8')
    const assetRe = /(?:src|href)="(\/static\/[^"]+\.(?:js|css))"/g
    let totalGzip = 0
    let assetCount = 0
    for (const match of html.matchAll(assetRe)) {
      const file = path.resolve(process.cwd(), 'dist', match[1].replace(/^\//, ''))
      if (!existsSync(file)) continue
      totalGzip += gzipSync(readFileSync(file)).length
      assetCount += 1
    }

    // 至少含 index.js（同步入口）；否则 index.html 解析或构建异常。
    expect(assetCount).toBeGreaterThan(0)
    // 2026-10-12 实测 ≈ 721 KB。此用例是「图标数据被误提升进首屏」的哨兵：
    // 治理时先把图标改成静态引入，首屏立刻涨到 739 KB（react-icons 核心 +
    // 49 个图标模块被拉进首个同步共享 chunk），改为单次懒加载后回到 721 KB。
    // 740 KB 上限（+2.6%）正好能把那次 +18 KB 的回归拦下，又不因路由微调误报。
    expect(totalGzip).toBeLessThan(740 * 1024)
  })

  it('largest async chunk stays under 4 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    const maxChunk = Math.max(...files.filter((f) => !f.name.startsWith('index.')).map((f) => f.sizeBytes))
    // 实测最大 async chunk = vendor-charts ≈ 2.2 MB（图表库，与图标治理无关）。
    // 旧上限 8 MB 在治理后形同虚设；4 MB 仍给图表库留近一倍余量，同时能拦住
    // 任何把图标整包或新重依赖塞进单个 async chunk 的回归。
    expect(maxChunk).toBeLessThan(4 * 1024 * 1024)
  })

  it('routes are code-split into separate async chunks, not all bundled into index', () => {
    const files = jsFiles()
    if (files.length === 0) return
    // 非入口 chunk 存在多个 => 路由/页面级 code-split 生效。
    // 若新页面全打进 index，此处会暴露为仅剩少量 vendor chunk。
    const nonIndex = files.filter((f) => !f.name.startsWith('index.'))
    expect(nonIndex.length).toBeGreaterThanOrEqual(2)
    // §4.2.5 P2-3 加固：路由级异步 chunk（async/ 子目录）必须达到下限，
    // 否则既有 vendor 拆分会掩盖「新页面全打进 index」的回归。
    const asyncRouteChunks = files.filter((f) => path.dirname(f.name) === 'async')
    expect(asyncRouteChunks.length).toBeGreaterThanOrEqual(10)
  })

  it('known heavy vendor chunks stay under 4 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    // 已知重 vendor（charts/shiki/editor）各自独立成 chunk 时须在预算内；
    // 未独立成 chunk（未被引用）则跳过，避免对构建产物做硬性存在断言。
    // 实测 vendor-charts ≈ 2.2 MB、vendor-editor ≈ 0.5 MB、vendor-shiki 未独立成块。
    for (const prefix of ['vendor-charts', 'vendor-shiki', 'vendor-editor']) {
      const chunk = files.find((f) => path.basename(f.name).startsWith(prefix))
      if (!chunk) continue
      expect(chunk.sizeBytes).toBeLessThan(4 * 1024 * 1024)
    }
  })
})
