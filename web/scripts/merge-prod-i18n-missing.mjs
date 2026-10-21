/**
 * 把「生产环境首屏缺的 key」并回 common-keys.json。
 * 生产是首屏文案的最终真相（本地 mock 不会渲染 OAuth 按钮、模型列表等）。
 * 用法：先建好 9600 隧道，然后 node scripts/merge-prod-i18n-missing.mjs
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'

const WEB = 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/web'
const KEYS = WEB + '/src/i18n/common-keys.json'

let out = ''
try {
  out = execFileSync('node', ['e2e-local/prod-i18n-no-flash.cjs'], { cwd: WEB, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 })
} catch (error) {
  out = (error.stdout || '') + (error.stderr || '')
}
const start = out.indexOf('[')
const end = out.lastIndexOf(']')
if (start < 0 || end <= start) {
  console.error(out.slice(-800))
  process.exit(1)
}
const report = JSON.parse(out.slice(start, end + 1))
const missing = [...new Set(report.flatMap((r) => r.realMissing || []))].filter(Boolean)
console.log('production missing keys:', missing.length)

const current = JSON.parse(readFileSync(KEYS, 'utf8'))
const merged = [...new Set([...current, ...missing])].sort()
writeFileSync(KEYS, JSON.stringify(merged, null, 2) + '\n')
console.log(`common-keys ${current.length} -> ${merged.length}`)
console.log(execFileSync('node', ['scripts/build-i18n-common.mjs'], { cwd: WEB, encoding: 'utf8' }).trim().split('\n').pop())
