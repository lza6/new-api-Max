/**
 * 生成首屏 i18n key 子集：`web/src/i18n/common-keys.json`
 *
 * 为什么需要：7 份语言包各 519–754KB raw / 168–195KB gzip，而 `main.tsx` 会
 * `await i18nReady` 之后才 `root.render()` —— 首屏被整包语言包阻塞。
 * 首屏实际用到的 key 只占 4% 左右，把这部分拆成 `<lang>.common.json` 先加载，
 * 其余在首屏渲染后后台补齐，可以把阻塞体积从 ~175KB gzip 降到 ~8KB。
 *
 * 本脚本只做**静态扫描**；动态 key（`t(variable)`）会漏。因此落地后必须用
 * `web/e2e-local/i18n-no-flash.cjs` 在真实浏览器里检测「渲染文本是否等于英文 key」，
 * 把漏掉的 key 补回 common-keys.json 再重跑，直到零命中。
 *
 * 用法：node scripts/scan-i18n-common.mjs [--merge]
 *   --merge 与已有 common-keys.json 取并集（迭代补漏时用）
 */
import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

const root = process.cwd()
const OUT = join(root, 'src/i18n/common-keys.json')

/** 首屏可达的目录：壳层 + 公开页 + 登录 + 错误页 + 它们的配置。 */
const DIRS = [
  'src/components',
  'src/features/home',
  'src/features/auth',
  'src/features/pricing',
  'src/features/status',
  'src/features/errors',
  'src/features/about',
  'src/features/legal',
  'src/features/docs',
  'src/features/setup',
  'src/lib',
  'src/hooks',
  'src/context',
  'src/routes',
]

const keys = new Set()
const reT = /(?:^|[^A-Za-z0-9_$.])t\(\s*'((?:[^'\\]|\\.){2,300})'\s*[,)]/gs
const reKey = /[A-Za-z]*[Kk]ey:\s*'([^'\\]{2,300})'/g

function walk(dir) {
  const full = join(root, dir)
  if (!existsSync(full)) return
  for (const entry of readdirSync(full, { withFileTypes: true })) {
    const p = join(full, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'node_modules') continue
      walk(p)
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      const s = readFileSync(p, 'utf8')
      for (const m of s.matchAll(reT)) keys.add(m[1].replace(/\\'/g, "'"))
      for (const m of s.matchAll(reKey)) keys.add(m[1])
    }
  }
}

for (const d of DIRS) walk(d)

const zh = JSON.parse(readFileSync(join(root, 'src/i18n/locales/zh.json'), 'utf8')).translation
let list = [...keys].filter((k) => k in zh).sort()

if (process.argv.includes('--merge') && existsSync(OUT)) {
  const previous = JSON.parse(readFileSync(OUT, 'utf8'))
  list = [...new Set([...previous, ...list])].sort()
}

writeFileSync(OUT, JSON.stringify(list, null, 2) + '\n')
console.log(`[scan-i18n-common] ${list.length} keys -> src/i18n/common-keys.json`)
