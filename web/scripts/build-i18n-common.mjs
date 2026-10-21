/**
 * 从 `src/i18n/common-keys.json` 生成 7 份 `<lang>.common.json`。
 *
 * 这些文件是**生成产物**，不要手改；改 key 走 `scan-i18n-common.mjs --merge`
 * 或直接编辑 common-keys.json 后重跑本脚本。
 *
 * 用法：node scripts/build-i18n-common.mjs（`bun run i18n:common`）
 */
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { gzipSync } from 'node:zlib'

const root = process.cwd()
const keysPath = join(root, 'src/i18n/common-keys.json')
const LOCALES = {
  en: 'en.json',
  zhCN: 'zh.json',
  zhTW: 'zh-TW.json',
  fr: 'fr.json',
  ru: 'ru.json',
  ja: 'ja.json',
  vi: 'vi.json',
}

if (!existsSync(keysPath)) {
  console.error('[i18n:common] src/i18n/common-keys.json missing — run scan-i18n-common.mjs first')
  process.exit(1)
}

const keys = JSON.parse(readFileSync(keysPath, 'utf8'))
if (!Array.isArray(keys) || keys.length === 0) {
  console.error('[i18n:common] common-keys.json must be a non-empty array')
  process.exit(1)
}

let totalGzip = 0
for (const [code, file] of Object.entries(LOCALES)) {
  const full = JSON.parse(readFileSync(join(root, 'src/i18n/locales', file), 'utf8')).translation
  const common = {}
  let missing = 0
  for (const key of keys) {
    if (key in full) {
      common[key] = full[key]
    } else {
      missing += 1
    }
  }
  const out = join(root, 'src/i18n/locales', file.replace(/\.json$/, '.common.json'))
  const json = JSON.stringify({ translation: common })
  writeFileSync(out, json + '\n')
  const gz = gzipSync(Buffer.from(json)).length
  totalGzip += gz
  console.log(
    `[i18n:common] ${code.padEnd(5)} ${Object.keys(common).length} keys, ${(gz / 1024).toFixed(1)}KB gzip` +
      (missing ? ` (${missing} keys missing in ${file})` : '')
  )
}
console.log(`[i18n:common] done, all locales ${(totalGzip / 1024).toFixed(1)}KB gzip total`)
