/**
 * 术语门禁（尼尔森 H2.4 / H4.5）：
 * 含 `quota` 的文案必须统一说「额度」，不得混用「配额 / 容量」。
 *
 * 例外：限流/并发类 key（rate / limit / concurrency）里「配额」是更准确的词，跳过。
 *
 * 用法：node scripts/check-terminology.mjs（`bun run i18n:terminology`）
 */
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const root = process.cwd()
const LOCALES = ['zh.json', 'zh-TW.json']
const BANNED = /配额|容量|配額/
// Word boundaries matter: the previous /rate|limit|concurren/ matched the
// "limit" inside "Unlimited", so "Unlimited Quota => 无限配额" was exempted and
// the gate reported PASS while two violations were live. A gate that cannot see
// the defect it was written for is worse than no gate.
const EXEMPT_KEY = /\b(rate|rates|limit|limits|limited|concurrency|concurrent)\b/i

let violations = 0
for (const file of LOCALES) {
  const translation = JSON.parse(readFileSync(join(root, 'src/i18n/locales', file), 'utf8')).translation
  for (const [key, value] of Object.entries(translation)) {
    if (!/quota/i.test(key) || EXEMPT_KEY.test(key)) {continue}
    if (typeof value === 'string' && BANNED.test(value)) {
      violations += 1
      console.error(`[terminology] ${file}: "${key}" -> "${value}"`)
    }
  }
}

if (violations > 0) {
  console.error(`[terminology] FAIL: ${violations} 处配额/容量混用（quota 应统一为「额度」）`)
  process.exit(1)
}
console.log('[terminology] PASS: quota 相关文案已统一为「额度」')
