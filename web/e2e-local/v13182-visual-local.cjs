/**
 * v1.3.182 视觉批次本地 E2E：真实浏览器截图 + axe 可访问性核查。
 *
 * 用法（先起本地后端 + dev server）：
 *   BASE=http://127.0.0.1:5173 USER=root PASS=123456 \
 *     node e2e-local/v13182-visual-local.cjs
 *
 * 产物：e2e-local/v13182/<page>-<width>-<theme>.png + results.json
 */
const fs = require('node:fs')
const path = require('node:path')

const { chromium } = require('playwright')

const BASE = process.env.BASE || 'http://127.0.0.1:5173'
const USER = process.env.USER_NAME || 'root'
const PASS = process.env.USER_PASS || '123456'
const OUT = path.join(__dirname, 'v13182')

const PAGES = [
  { name: 'dashboard-overview', route: '/dashboard/overview' },
  { name: 'dashboard-models', route: '/dashboard/models' },
  { name: 'keys', route: '/keys' },
]
const VIEWPORTS = [
  { width: 1440, height: 900 },
  { width: 768, height: 1024 },
  { width: 320, height: 800 },
]

;(async () => {
  fs.mkdirSync(OUT, { recursive: true })
  const browser = await chromium.launch()
  const results = { base: BASE, generatedAt: new Date().toISOString(), pages: [] }

  const context = await browser.newContext({
    viewport: VIEWPORTS[0],
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') {consoleErrors.push(msg.text().slice(0, 300))}
  })
  page.on('pageerror', (err) => consoleErrors.push('pageerror: ' + String(err).slice(0, 300)))

  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('input[name="username"]', { timeout: 45000 })
  await page.fill('input[name="username"]', USER)
  await page.fill('input[name="password"]', PASS)
  await page.click('button[type="submit"]')
  await page.waitForTimeout(5000)
  results.afterLoginUrl = page.url()

  for (const target of PAGES) {
    for (const viewport of VIEWPORTS) {
      await page.setViewportSize(viewport)
      await page.goto(`${BASE}${target.route}`, { waitUntil: 'domcontentloaded', timeout: 60000 })
      await page.waitForTimeout(3500)
      const file = path.join(OUT, `${target.name}-${viewport.width}-light.png`)
      await page.screenshot({ path: file, fullPage: false })
      results.pages.push({ page: target.name, width: viewport.width, theme: 'light', file })
    }
  }

  // 暗色：令牌由 .dark 根类驱动，直接切类后复拍一张
  await page.setViewportSize(VIEWPORTS[0])
  await page.goto(`${BASE}/dashboard/overview`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForTimeout(2500)
  await page.evaluate(() => document.documentElement.classList.add('dark'))
  await page.waitForTimeout(800)
  const darkFile = path.join(OUT, 'dashboard-overview-1440-dark.png')
  await page.screenshot({ path: darkFile, fullPage: false })
  results.pages.push({ page: 'dashboard-overview', width: 1440, theme: 'dark', file: darkFile })

  // axe：把 axe-core 注入真实页面跑一遍，记录违规
  const axePath = require.resolve('axe-core/axe.min.js')
  await page.evaluate(() => document.documentElement.classList.remove('dark'))
  await page.addScriptTag({ path: axePath })
  const axe = await page.evaluate(async () => {
    const result = await window.axe.run(document, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] },
    })
    return {
      violations: result.violations.map((v) => ({
        id: v.id,
        impact: v.impact,
        nodes: v.nodes.length,
        help: v.help,
      })),
    }
  })
  results.axe = axe
  results.consoleErrors = consoleErrors.slice(0, 20)

  fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify(results, null, 2))
  console.log('after-login-url=' + results.afterLoginUrl)
  console.log('screenshots=' + results.pages.length)
  console.log('axe-violations=' + axe.violations.length)
  if (axe.violations.length) {console.log(JSON.stringify(axe.violations, null, 2))}
  console.log('console-errors=' + consoleErrors.length)
  if (consoleErrors.length) {console.log(consoleErrors.slice(0, 5).join('\n'))}
  await browser.close()
})().catch((err) => {
  console.error('ERR=' + err.message)
  process.exit(1)
})
