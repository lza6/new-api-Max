/**
 * N11：生产环境 axe 审计（WCAG 2.1 A/AA）。
 * 需要 9600 隧道；管理员页用 PROD_VERIFY_USER。
 *
 * 用法：PROD_VERIFY_USER=xxx node e2e-local/prod-axe-audit.cjs
 * 输出：每条违规的 id / impact / 命中节点数 / 首个选择器
 */
const path = require('node:path')
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER
const PASS = process.env.PROD_VERIFY_PASS || 'TempVerify!2026#Qz'
const PUBLIC_PATHS = ['/', '/pricing', '/status', '/sign-in']
const ADMIN_PATHS = ['/dashboard/models', '/system-settings/site/system-info', '/wallet', '/keys']

async function audit(page, label) {
  await page.addScriptTag({ path: path.join(process.cwd(), 'node_modules/axe-core/axe.min.js') })
  const result = await page.evaluate(async () => {
    const r = await window.axe.run(document, {
      runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'] },
      resultTypes: ['violations'],
    })
    return r.violations.map((v) => ({
      id: v.id,
      impact: v.impact,
      help: v.help,
      nodes: v.nodes.length,
      target: v.nodes[0]?.target?.join(' ') ?? '',
      summary: (v.nodes[0]?.failureSummary ?? '').slice(0, 140),
    }))
  })
  return { label, violations: result }
}

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } })
  const page = await context.newPage()
  const out = []

  for (const p of PUBLIC_PATHS) {
    await page.goto(BASE + p, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(3000)
    out.push(await audit(page, p))
  }

  if (USER) {
    await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForSelector('input[name=username]', { timeout: 30000 })
    await page.fill('input[name=username]', USER)
    await page.fill('input[name=password]', PASS)
    await page.click('button[type=submit]')
    await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 })
    for (const p of ADMIN_PATHS) {
      await page.goto(BASE + p, { waitUntil: 'domcontentloaded', timeout: 60000 })
      await page.waitForTimeout(4000)
      out.push(await audit(page, p))
    }
  }

  await browser.close()
  const totals = {}
  for (const pageResult of out) {
    for (const v of pageResult.violations) {
      totals[v.id] = totals[v.id] ?? { id: v.id, impact: v.impact, nodes: 0, pages: [] }
      totals[v.id].nodes += v.nodes
      if (!totals[v.id].pages.includes(pageResult.label)) totals[v.id].pages.push(pageResult.label)
    }
  }
  const ranked = Object.values(totals).sort((a, b) => b.nodes - a.nodes)
  console.log(JSON.stringify({ perPage: out, ranked }, null, 1))

  const serious = ranked.filter((v) => v.impact === 'critical' || v.impact === 'serious')
  if (serious.length) {
    console.error(`\nFAIL: ${serious.length} 类 critical/serious 违规`)
    process.exit(1)
  }
  console.log('\nPASS: 无 critical/serious 违规')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
