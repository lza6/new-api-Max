/**
 * 「不闪英文」验收（对照法）
 *
 * 目的：确认「首屏只加载 common 子集」时不会出现英文原文闪切。
 *
 * 方法：同一页面跑两次
 *   A. 中止所有 `i18n-full-*` chunk（模拟首屏窗口）
 *   B. 正常加载（整包语言包到位）
 * 判定：
 *   - 若某文本在 A 中是**英文 key 原文**、而在 B 中变成中文 → 说明它依赖整包，
 *     属于**闪切**，必须补进 common-keys.json。
 *   - 若在 B 中仍然是英文原文 → 该文案本来就是硬编码/中英同形（如 Anthropic、
 *     CC Switch），与本次拆分无关，不算闪切。
 *
 * 用法：node e2e-local/i18n-no-flash.cjs
 */
const http = require('node:http')
const fs = require('node:fs')
const p = require('node:path')
const z = require('node:zlib')

const ROOT = p.join(process.cwd(), 'dist')
const PORT = 4188
const MIME = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.woff2': 'font/woff2', '.png': 'image/png', '.svg': 'image/svg+xml', '.ico': 'image/x-icon' }
const gz = new Map()
;(function pre(d) {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const f = p.join(d, e.name)
    if (e.isDirectory()) pre(f)
    else if (/\.(js|css|html|json|svg)$/.test(e.name)) gz.set(f, z.gzipSync(fs.readFileSync(f)))
  }
})(ROOT)
const srv = http.createServer((req, res) => {
  const u = decodeURIComponent((req.url || '/').split('?')[0])
  let f = p.join(ROOT, u)
  if (!f.startsWith(ROOT)) return res.writeHead(403).end()
  if (!fs.existsSync(f) || fs.statSync(f).isDirectory()) f = p.join(ROOT, 'index.html')
  const b = gz.get(f)
  if (/\bgzip\b/.test(req.headers['accept-encoding'] || '') && b) {
    res.writeHead(200, { 'Content-Type': MIME[p.extname(f)] || 'application/octet-stream', 'Content-Encoding': 'gzip' })
    return res.end(b)
  }
  res.writeHead(200, { 'Content-Type': MIME[p.extname(f)] || 'application/octet-stream' })
  res.end(fs.readFileSync(f))
})

const enKeys = new Set(Object.keys(JSON.parse(fs.readFileSync('src/i18n/locales/en.json', 'utf8')).translation))
const PATHS = ['/', '/sign-in', '/pricing', '/status']

async function collect(browser, path, blockFull) {
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
  const page = await context.newPage()
  await page.route('**/api/**', (r) => {
    const u = r.request().url()
    if (u.includes('/api/setup')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"status":true}}' })
    if (u.includes('/api/notice')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":""}' })
    if (u.includes('/api/home_page_content')) return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":""}' })
    return r.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"system_name":"B"}}' })
  })
  if (blockFull) await page.route(/i18n-full-/, (r) => r.abort())
  await page.goto(`http://127.0.0.1:${PORT}${path}`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 30000 }).catch(() => {})
  await page.waitForTimeout(blockFull ? 1200 : 3000)
  const lines = await page.evaluate(() => {
    const out = []
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    let node
    while ((node = walker.nextNode())) {
      const text = (node.textContent || '').trim()
      if (text.length >= 3 && text.length <= 200) out.push(text)
    }
    return [...new Set(out)]
  })
  await context.close()
  return lines
}

;(async () => {
  const { chromium } = require('playwright')
  await new Promise((r) => srv.listen(PORT, r))
  const browser = await chromium.launch()
  const report = []
  for (const path of PATHS) {
    const onlyCommon = await collect(browser, path, true)
    const full = await collect(browser, path, false)
    const englishInA = onlyCommon.filter((l) => enKeys.has(l))
    const englishInB = new Set(full.filter((l) => enKeys.has(l)))
    // 闪切 = 子集下是英文原文，但整包到位后变成中文
    const flash = englishInA.filter((l) => !englishInB.has(l))
    report.push({ path, linesCommon: onlyCommon.length, linesFull: full.length, englishInA: englishInA.length, flash })
  }
  await browser.close()
  srv.close()
  console.log(JSON.stringify(report, null, 1))
  const bad = report.filter((r) => r.flash.length > 0)
  if (bad.length) {
    console.error('\nFAIL: 存在依赖整包语言包的首屏文案（补进 common-keys.json 后重跑）')
    process.exit(1)
  }
  console.log('\nPASS: 4 条首屏路由在仅有 common 子集时无英文闪切（其余英文为中英同形/硬编码，与拆分无关）')
})().catch((e) => { console.error(e); srv.close(); process.exit(1) })
