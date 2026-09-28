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
import { readdirSync, statSync, existsSync } from 'node:fs'
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
  it('total JS stays under 70 MB raw / 25 MB gzip-estimated', () => {
    const files = jsFiles()
    if (files.length === 0) {
      return // dist 未构建（CI 单测阶段），跳过
    }
    const totalRaw = files.reduce((s, f) => s + f.sizeBytes, 0)
    expect(totalRaw).toBeLessThan(70 * 1024 * 1024)
    expect(gzipEstimate(totalRaw)).toBeLessThan(25 * 1024 * 1024)
  })

  it('index entry chunk stays under 5 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    const index = files.find((f) => f.name.startsWith('index.'))
    expect(index).toBeTruthy()
    // no-non-null-assertion 禁止 `!`：前置 toBeTruthy 已保证存在，此处用可选链。
    expect(index?.sizeBytes ?? 0).toBeLessThan(5 * 1024 * 1024)
  })

  it('largest async chunk stays under 8 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    const maxChunk = Math.max(...files.filter((f) => !f.name.startsWith('index.')).map((f) => f.sizeBytes))
    expect(maxChunk).toBeLessThan(8 * 1024 * 1024)
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

  it('known heavy vendor chunks stay under 8 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    // 已知重 vendor（charts/shiki/editor）各自独立成 chunk 时须在预算内；
    // 未独立成 chunk（未被引用）则跳过，避免对构建产物做硬性存在断言。
    for (const prefix of ['vendor-charts', 'vendor-shiki', 'vendor-editor']) {
      const chunk = files.find((f) => path.basename(f.name).startsWith(prefix))
      if (!chunk) continue
      expect(chunk.sizeBytes).toBeLessThan(8 * 1024 * 1024)
    }
  })
})
