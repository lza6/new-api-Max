/* eslint-disable typescript/no-require-imports, no-console -- standalone Node probe against a live site via SSH tunnel */
/**
 * 线上站点探针（真实浏览器，经 SSH 隧道直连主站 new-api）
 * 用法：node v1383-prod-live-check.cjs [baseUrl]   默认 http://127.0.0.1:9600
 * 断言：标题拼接、FAQ 区块 + JSON-LD、WebGL/CSS 视觉非空、无控制台错误、无横向溢出。
 */
const path = require('node:path')
const fs = require('node:fs')
const { chromium } = require('playwright')

const BASE = process.argv[2] || 'http://127.0.0.1:9600'
// Optional host-remap for tunneled targets whose TLS cert does not match the
// local endpoint, e.g. PW_HOSTMAP="japi.tingfengai.art 127.0.0.1".
const HOSTMAP = process.env.PW_HOSTMAP
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.83');

(async () => {
  const browser = await chromium.launch(
    HOSTMAP
      ? { args: [`--host-resolver-rules=MAP ${HOSTMAP}`] }
      : {}
  )
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'no-preference',
    ignoreHTTPSErrors: true,
  })
  await context.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const page = await context.newPage()
  const errors = []
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text())
  })

  const results = []
  const rec = (n, ok, d) => {
    results.push({ n, ok, d })
    console.log(`${ok ? '✅' : '❌'} ${n}${d ? ` — ${d}` : ''}`)
  }

  try {
    await page.goto(BASE, { waitUntil: 'networkidle', timeout: 60000 })
    // trigger reveals
    const h = await page.evaluate(() => document.body.scrollHeight)
    for (let y = 0; y < h; y += 700) {
      await page.evaluate((t) => window.scrollTo(0, t), y)
      await page.waitForTimeout(70)
    }
    await page.waitForTimeout(800)

    const title = await page.title()
    rec('标题含品牌+关键词后缀', /·/.test(title) && /API/.test(title), title)

    const h1 = await page.locator('h1').first().innerText().catch(() => '')
    rec('Hero 标题渲染', h1.includes('Unified API Gateway'), h1.replace(/\s+/g, ' ').slice(0, 50))

    const faqVisible = await page.getByText('Frequently asked questions').first().isVisible().catch(() => false)
    rec('FAQ 区块可见', faqVisible)

    const ld = await page.evaluate(() => {
      const el = document.getElementById('landing-faq-jsonld')
      try { return el ? JSON.parse(el.textContent) : null } catch { return null }
    })
    rec('FAQPage JSON-LD 注入', Boolean(ld && ld['@type'] === 'FAQPage'), ld ? `count=${ld.mainEntity?.length}` : 'missing')

    const canvas = await page.locator('canvas').count()
    const fallback = (await page.getByText('Smart Routing').count()) + (await page.getByText('Transparent Billing').count())
    rec('Hero 视觉区非空（WebGL 或 CSS）', canvas > 0 || fallback > 0, `canvas=${canvas} fallback=${fallback}`)

    const ov = await page.evaluate(() => ({ s: document.documentElement.scrollWidth, c: document.documentElement.clientWidth }))
    rec('无横向溢出', ov.s <= ov.c + 2, `scroll=${ov.s} client=${ov.c}`)

    rec('无控制台错误', errors.length === 0, errors.slice(0, 3).join(' | '))

    fs.mkdirSync(OUT, { recursive: true })
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(200)
    await page.screenshot({ path: path.join(OUT, 'prod-home-light.png'), fullPage: true })
  } catch (err) {
    rec('脚本异常', false, String(err))
  } finally {
    await browser.close()
  }

  fs.mkdirSync(OUT, { recursive: true })
  fs.writeFileSync(path.join(OUT, 'prod-live-results.json'), JSON.stringify({ base: BASE, results }, null, 2))
  const failed = results.filter((r) => !r.ok)
  console.log(`\n${results.length - failed.length}/${results.length} passed (live: ${BASE})`)
  process.exit(failed.length ? 1 : 0)
})()
