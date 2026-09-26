/**
 * T9-4 性能预算回归测试
 *
 * 读取生产构建产物（web/dist/static/js）验证体积预算，防止依赖回归导致
 * 入口/总包体积爆炸。CI 顺序：bun run build → bunx vitest run（本文件）。
 * 若 dist 不存在（纯单测阶段），测试跳过而非失败。
 */
import { readdirSync, statSync, existsSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'

const distJsDir = path.resolve(process.cwd(), 'dist/static/js')

function jsFiles(): { name: string; sizeBytes: number }[] {
  if (!existsSync(distJsDir)) return []
  return readdirSync(distJsDir)
    .filter((f) => f.endsWith('.js'))
    .map((f) => ({ name: f, sizeBytes: statSync(path.join(distJsDir, f)).size }))
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
    const index = files.find((f) => /^index\./.test(f.name))
    expect(index).toBeTruthy()
    expect(index!.sizeBytes).toBeLessThan(5 * 1024 * 1024)
  })

  it('largest async chunk stays under 8 MB raw', () => {
    const files = jsFiles()
    if (files.length === 0) return
    const maxChunk = Math.max(...files.filter((f) => !/^index\./.test(f.name)).map((f) => f.sizeBytes))
    expect(maxChunk).toBeLessThan(8 * 1024 * 1024)
  })
})
