/**
 * 生产环境「不闪英文」验收：阻断整包语言包，读生产页面里 i18next 报告的缺失 key。
 * 需要 9600 隧道（ssh -N -L 9600:127.0.0.1:443）。
 */
const { chromium } = require('playwright')
const fs = require('node:fs')

const BASE = 'https://freeapi.tingfengai.art:9600'
const zhKeys = new Set(Object.keys(JSON.parse(fs.readFileSync('src/i18n/locales/zh.json', 'utf8')).translation))
const PATHS = ['/', '/sign-in', '/sign-up', '/pricing', '/status', '/about', '/docs', '/model-test', '/tool-setup', '/user-agreement', '/privacy-policy', '/rankings']

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const report = []
  for (const path of PATHS) {
    const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })
    const page = await context.newPage()
    await page.route(/i18n-full-/, (r) => r.abort())
    await page.goto(BASE + path, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 30000 }).catch(() => {})
    await page.waitForTimeout(2000)
    const missing = await page.evaluate(() => window.__i18nMissing ?? [])
    report.push({ path, missing, realMissing: missing.filter((k) => zhKeys.has(k)) })
    await context.close()
  }
  await browser.close()
  console.log(JSON.stringify(report, null, 1))
  const real = [...new Set(report.flatMap((r) => r.realMissing))]
  if (real.length) {
    console.error('FAIL: production first paint is missing ' + real.length + ' translated keys')
    process.exit(1)
  }
  console.log('PASS: 生产首屏在只有 common 子集时无任何可翻译 key 缺失（不闪英文）')
})().catch((e) => { console.error(e); process.exit(1) })
