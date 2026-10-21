/**
 * 迭代补漏：跑 `i18n-common-audit` → 把 i18next 报告的缺失 key 并进
 * `common-keys.json` → 重建 common 包 → 重新构建 → 再测。最多 4 轮。
 */
import { execFileSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'

const WEB = 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/web'
const KEYS = WEB + '/src/i18n/common-keys.json'

function run(cmd, args) {
  try {
    return execFileSync(cmd, args, { cwd: WEB, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], maxBuffer: 64 * 1024 * 1024 })
  } catch (error) {
    return (error.stdout || '') + (error.stderr || '')
  }
}

for (let round = 1; round <= 4; round += 1) {
  const out = run('node', ['e2e-local/i18n-common-audit.cjs'])
  const start = out.indexOf('[')
  const end = out.lastIndexOf(']')
  const report = start >= 0 && end > start ? JSON.parse(out.slice(start, end + 1)) : []
  const missing = [...new Set(report.flatMap((r) => r.missing || []))].filter(Boolean)
  console.log(`[round ${round}] missing keys: ${missing.length}`)
  if (missing.length === 0) {
    console.log('OK: common bundle covers all first-paint keys')
    process.exit(0)
  }
  const current = JSON.parse(readFileSync(KEYS, 'utf8'))
  const merged = [...new Set([...current, ...missing])].sort()
  if (merged.length === current.length) {
    console.error('[fix] keys unchanged but audit still failing — inspect manually:')
    console.error(JSON.stringify(report, null, 1))
    process.exit(1)
  }
  writeFileSync(KEYS, JSON.stringify(merged, null, 2) + '\n')
  console.log(`[round ${round}] common-keys ${current.length} -> ${merged.length}`)
  console.log(run('node', ['scripts/build-i18n-common.mjs']).trim().split('\n').pop())
  const build = run('bun', ['run', 'build'])
  console.log(`[round ${round}] build ${/\[stamp-sw\]/.test(build) ? 'OK' : 'FAILED'}`)
  if (!/\[stamp-sw\]/.test(build)) {
    console.log(build.slice(-1500))
    process.exit(1)
  }
}
console.error('still missing after 4 rounds')
process.exit(1)
