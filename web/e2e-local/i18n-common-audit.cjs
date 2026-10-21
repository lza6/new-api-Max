/**
 * 首屏 i18n 覆盖审计：只放行 common 子集，读取 i18next 真正报告缺失的 key。
 *
 * 与基于文本比对的版本相比，这里直接消费 `missingKeyHandler` 写入的
 * `window.__i18nMissing`，不会因为 innerText 换行/裁剪而漏判。
 *
 * 用法：node e2e-local/i18n-common-audit.cjs
 * 输出：JSON（每页 missing 列表）；退出码 0 = 无缺失
 */
const http = require('node:http')
const fs = require('node:fs')
const p = require('node:path')
const z = require('node:zlib')

const ROOT = p.join(process.cwd(), 'dist')
const PORT = 4190
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.woff2': 'font/woff2', '.png': 'image/png', '.svg': 'image/svg+xml', '.ico': 'image/x-icon' }
const gz = new Map()
;(function pre(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const f = p.join(d, e.name)
    if (e.isDirectory()) pre(f)
    else if (/\.(js|css|html|json|svg)$/.test(e.name)) gz.set(f, z.gzipSync(fs.readFileSync(f)))
  }
})(ROOT)
const srv = http.createServer((q, s) => {
  const u = decodeURIComponent((q.url || '/').split('?')[0])
  let f = p.join(ROOT, u)
  if (!f.startsWith(ROOT)) return s.writeHead(403).end()
  if (!fs.existsSync(f) || fs.statSync(f).isDirectory()) f = p.join(ROOT, 'index.html')
  const b = gz.get(f)
  if (/\bgzip\b/.test(q.headers['accept-encoding'] || '') && b) {
    s.writeHead(200, { 'Content-Type': MIME[p.extname(f)] || 'application/octet-stream', 'Content-Encoding': 'gzip' })
    return s.end(b)
  }
  s.writeHead(200, { 'Content-Type': MIME[p.extname(f)] || 'application/octet-stream' })
  s.end(fs.readFileSync(f))
})

const zhKeys = new Set(
  Object.keys(JSON.parse(fs.readFileSync('src/i18n/locales/zh.json', 'utf8')).translation)
)
const PATHS = ['/', '/sign-in', '/pricing', '/status']

;(async () => {
  const { chromium } = require('playwright')
  await new Promise((r) => srv.listen(PORT, r))
  const browser = await chromium.launch()
  const report = []
  for (const path of PATHS) {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
    const page = await context.newPage()
    await page.route('**/api/**', (r) => {
      const u = r.request().url()
      if (u.includes('/api/setup')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"status":true}}' })
      if (u.includes('/api/notice')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":""}' })
      if (u.includes('/api/home_page_content')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":""}' })
      return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"system_name":"B"}}' })
    })
    // 只允许 common：整包一律不送达
    await page.route(/i18n-full-/, (r) => r.abort())
    await page.goto(`http://127.0.0.1:${PORT}${path}`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 30000 }).catch(() => {})
    await page.waitForTimeout(1500)
    const missing = await page.evaluate(() => window.__i18nMissing ?? [])
    report.push({ path, missing, realMissing: missing.filter((k) => zhKeys.has(k)) })
    await context.close()
  }
  await browser.close()
  srv.close()
  console.log(JSON.stringify(report, null, 1))
  const all = [...new Set(report.flatMap((r) => r.realMissing))]
  // 代码里直接写中文的 `t('主页')` 这类调用在任何一份语言包里都不存在，
  // 中文用户看到中文、英文用户也看到中文 —— 属于既有 i18n 缺陷，不是拆分引入的闪切。
  const preExisting = [...new Set(report.flatMap((r) => r.missing))].filter((k) => !zhKeys.has(k))
  if (preExisting.length) {
    console.log(`pre-existing untranslatable keys (not a regression): ${preExisting.length}`)
    console.log(JSON.stringify(preExisting))
  }
  if (all.length) {
    console.error(`\nFAIL: ${all.length} keys missing from the common bundle`)
    process.exit(1)
  }
  console.log('\nPASS: common 子集覆盖 4 条首屏路由的全部 key')
})().catch((e) => { console.error(e); srv.close(); process.exit(1) })
