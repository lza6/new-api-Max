/**
 * 视觉基线（V12）：真实浏览器批量截图 + axe 核查，产物可直接做前后对比。
 *
 * 用法：
 *   BASE=http://127.0.0.1:5173 USER_PASS=*** node e2e-local/visual-baseline.cjs
 *
 * 环境变量：
 *   BASE        前端地址（默认 http://127.0.0.1:5173）
 *   USER_NAME   登录名（默认 root）
 *   USER_PASS   密码（默认 123456，仅本地测试用）
 *   PRESETS     逗号分隔的预设，默认 default,anthropic,ink
 *   PAGES       逗号分隔的 路径:名称，默认见 DEFAULT_PAGES
 *   WIDTHS      逗号分隔的断点，默认 1440,768,320
 *   DARK        1 = 额外拍一套暗色（默认 1）
 *   OUT         产物目录（默认 e2e-local/visual-baseline）
 */
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const BASE = process.env.BASE || 'http://127.0.0.1:5173'
const USER = process.env.USER_NAME || 'root'
const PASS = process.env.USER_PASS || '123456'
const PRESETS = (process.env.PRESETS || 'default,anthropic,ink').split(',')
const WIDTHS = (process.env.WIDTHS || '1440,768,320').split(',').map(Number)
const WITH_DARK = (process.env.DARK ?? '1') !== '0'
const OUT = path.resolve(process.env.OUT || path.join(__dirname, 'visual-baseline'))

const DEFAULT_PAGES = [
  '/dashboard/overview:dashboard-overview',
  '/dashboard/models:dashboard-models',
  '/keys:keys',
  '/wallet:wallet',
  '/channels:channels',
  '/usage-logs:usage-logs',
]
const PAGES = (process.env.PAGES || DEFAULT_PAGES.join(','))
  .split(',')
  .map((entry) => {
    const [route, name] = entry.split(':')
    return { route, name: name || route.replaceAll('/', '-').replace(/^-/, '') }
  })

const PRESET_COOKIE = 'theme_preset'

function slug(value) {
  return value.toLowerCase().replace(/[^a-z0-9-]/g, '-')
}

;(async () => {
  fs.mkdirSync(OUT, { recursive: true })
  const browser = await chromium.launch()
  const context = await browser.newContext({
    viewport: { width: WIDTHS[0], height: 900 },
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (msg) => {
    if (msg.type() === 'error') {consoleErrors.push(msg.text().slice(0, 240))}
  })
  page.on('pageerror', (err) => consoleErrors.push('pageerror: ' + String(err).slice(0, 240)))

  const results = {
    base: BASE,
    startedAt: new Date().toISOString(),
    presets: PRESETS,
    widths: WIDTHS,
    shots: [],
    axe: [],
  }

  await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded', timeout: 90000 })
  await page.waitForSelector('input[name="username"]', { timeout: 45000 })
  await page.fill('input[name="username"]', USER)
  await page.fill('input[name="password"]', PASS)
  await page.click('button[type="submit"]')
  await page.waitForTimeout(5000)
  results.afterLoginUrl = page.url()

  const axePath = require.resolve('axe-core/axe.min.js')

  for (const preset of PRESETS) {
    const cookieValue = preset === 'default' ? '' : preset
    await context.addCookies([
      {
        name: PRESET_COOKIE,
        value: cookieValue,
        url: BASE,
      },
    ])

    for (const target of PAGES) {
      for (const width of WIDTHS) {
        await page.setViewportSize({ width, height: width <= 400 ? 800 : 900 })
        await page.goto(`${BASE}${target.route}`, { waitUntil: 'domcontentloaded', timeout: 90000 })
        await page.waitForTimeout(2600)
        const file = path.join(
          OUT,
          `${slug(target.name)}-${width}-${slug(preset)}-light.png`
        )
        await page.screenshot({ path: file })
        results.shots.push({ page: target.name, preset, width, theme: 'light', file })

        if (width === WIDTHS[0]) {
          await page.addScriptTag({ path: axePath })
          const violations = await page.evaluate(async () => {
            const result = await window.axe.run(document, {
              runOnly: {
                type: 'tag',
                values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'],
              },
            })
            return result.violations.map((v) => ({
              id: v.id,
              impact: v.impact,
              nodes: v.nodes.length,
            }))
          })
          results.axe.push({ page: target.name, preset, violations })
        }
      }

      if (WITH_DARK) {
        await page.setViewportSize({ width: WIDTHS[0], height: 900 })
        await page.goto(`${BASE}${target.route}`, { waitUntil: 'domcontentloaded', timeout: 90000 })
        await page.waitForTimeout(2000)
        await page.evaluate(() => document.documentElement.classList.add('dark'))
        await page.waitForTimeout(600)
        const file = path.join(
          OUT,
          `${slug(target.name)}-${WIDTHS[0]}-${slug(preset)}-dark.png`
        )
        await page.screenshot({ path: file })
        results.shots.push({ page: target.name, preset, width: WIDTHS[0], theme: 'dark', file })
      }
    }
  }

  results.finishedAt = new Date().toISOString()
  results.consoleErrors = consoleErrors.slice(0, 20)
  results.axeViolationTotal = results.axe.reduce(
    (sum, entry) => sum + entry.violations.length,
    0
  )
  fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify(results, null, 2))

  console.log('shots=' + results.shots.length)
  console.log('axe-pages=' + results.axe.length + ' violations=' + results.axeViolationTotal)
  if (results.axeViolationTotal) {console.log(JSON.stringify(results.axe, null, 2))}
  console.log('console-errors=' + results.consoleErrors.length)
  await browser.close()
})().catch((err) => {
  console.error('ERR=' + err.message)
  process.exit(1)
})
