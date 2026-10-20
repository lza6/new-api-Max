// 生产 UI 真实验证：运营面板（利润/成本/覆盖率/成本规则）+ 隐私页文案。
// 通过 SSH 隧道 9600 → 生产 443（Caddy），域名映射到 127.0.0.1，走 HTTPS 以保留 Secure Cookie。
// 注意：Playwright 的 APIRequestContext 不走 --host-resolver-rules，所以必须用真实登录页登录。
const { chromium } = require('../web/node_modules/playwright')
const fs = require('fs')
const BASE = process.argv[2] || 'https://freeapi.tingfengai.art:9600'
const USER = process.argv[3]
const PASS = process.argv[4] || 'CaptureE2E!2026'
const OUT = '计划书/e2e-evidence/v1.3.162-ui'
let pass = 0, fail = 0
const results = []
function check(name, cond, detail) {
  results.push({ name, ok: !!cond, detail: detail || '' })
  if (cond) { pass++; console.log('  ✅ ' + name) } else { fail++; console.log('  ❌ ' + name + (detail ? ' — ' + detail : '')) }
}
async function waitAppReady(page) {
  // 等 React 真正挂载：真实用户路径下页面内容出现在 #root 内
  await page.waitForFunction(() => {
    const root = document.getElementById('root')
    return !!root && root.childElementCount > 0
  }, null, { timeout: 30000 }).catch(() => {})
  await page.waitForTimeout(1200)
}
async function uiLogin(context) {
  const p = await context.newPage()
  await p.goto('/sign-in', { waitUntil: 'domcontentloaded' })
  await p.locator('input[name="username"]').waitFor({ state: 'visible', timeout: 20000 })
  await p.locator('input[name="username"]').fill(USER)
  await p.locator('input[name="password"]').fill(PASS)
  const consent = p.locator('button[role="checkbox"], input[type="checkbox"]').first()
  if (await consent.count()) {
    const aria = await consent.getAttribute('aria-checked').catch(() => null)
    const isInput = (await consent.evaluate((el) => el.tagName === 'INPUT').catch(() => false))
    const checked = isInput ? await consent.isChecked().catch(() => false) : aria === 'true'
    if (!checked) await consent.click({ force: true }).catch(() => {})
  }
  const submit = p.locator('button[type="submit"]').first()
  if (await submit.isDisabled().catch(() => false)) console.log('  [login] submit disabled, retry consent')
  await submit.click({ timeout: 10000 })
  await p.waitForURL((u) => !u.pathname.includes('/sign-in'), { timeout: 15000 }).catch(() => {})
  await waitAppReady(p)
  const url = p.url()
  const txt = (await p.locator('body').innerText().catch(() => '')) || ''
  return { page: p, url, txt, ok: !url.includes('/sign-in') }
}
async function main() {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
  const login = await uiLogin(context)
  check('UI 登录（生产管理员）', login.ok, login.url + ' | ' + login.txt.slice(0, 120))

  const page = login.page
  const errors = []
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message))

  await page.goto('/ops-panel', { waitUntil: 'domcontentloaded' })
  await waitAppReady(page)
  // 冷启动时财务聚合可能耗时数秒：等利润卡片真正出现再断言，避免把「加载中」判成缺失
  await page.getByText(/利润与成本|Profit & Cost/).first().waitFor({ state: 'visible', timeout: 30000 }).catch(() => {})
  await page.waitForTimeout(1500)
  const text = (await page.locator('body').innerText().catch(() => '')) || ''
  fs.writeFileSync(OUT + '/debug-ops-body.txt', text)
  check('运营面板渲染', text.length > 200, 'len=' + text.length)
  check('利润与成本区块', /利润与成本|Profit & Cost/.test(text))
  check('今日/本月/本年财务数字', /本月|This Month/.test(text) && /本年|This Year/.test(text))
  check('覆盖率告警可见（营收未完全覆盖）', /成本覆盖率|Cost coverage/.test(text))
  check('建议每日上游充值', /建议每日上游充值|Recommended Daily Upstream Top-up/.test(text))
  check('成本规则表渲染', /模型成本|Model Costs/.test(text))
  for (const m of ['deepseek-v4.1-flash', 'step-5-preview', 'gpt-6-luna', 'glm-5.3-flash']) {
    check('成本规则可见: ' + m, text.includes(m))
  }
  check('添加成本规则入口', /添加成本规则|Add cost rule/.test(text))
  await page.screenshot({ path: OUT + '/ops-desktop-full.png', fullPage: true })
  // 页面滚动在内部容器里，scrollIntoView 必须用 DOM 原生方法，Playwright 的 locator 滚动不生效
  const scrollToText = async (needle) => {
    await page.evaluate((n) => {
      const all = Array.from(document.querySelectorAll('h1,h2,h3,h4,div,span'))
      const el = all.find((e) => e.textContent && e.textContent.trim().startsWith(n))
      if (el) el.scrollIntoView({ block: 'start' })
    }, needle)
    await page.waitForTimeout(600)
  }
  await scrollToText('利润与成本')
  await page.screenshot({ path: OUT + '/ops-profit-card.png' })
  await scrollToText('模型成本')
  await page.screenshot({ path: OUT + '/ops-cost-settings.png' })

  // 移动端 320px 横向溢出
  const mobile = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 320, height: 720 } })
  const mlogin = await uiLogin(mobile)
  check('320px 上下文 UI 登录', mlogin.ok, mlogin.url)
  const mp = mlogin.page
  await mp.goto('/ops-panel', { waitUntil: 'domcontentloaded' })
  await waitAppReady(mp)
  const overflow = await mp.evaluate(() => ({ sw: document.documentElement.scrollWidth, iw: window.innerWidth }))
  check('320px 运营面板无横向溢出', overflow.sw <= overflow.iw, JSON.stringify(overflow))
  await mp.screenshot({ path: OUT + '/ops-mobile-320.png', fullPage: true })

  // 隐私页文案（安全设置页 → 隐私卡片）。复用同一标签页，避免新标签页首帧鉴权竞态。
  const pp = page
  await pp.goto('/security', { waitUntil: 'domcontentloaded' })
  await waitAppReady(pp)
  const ptext = (await pp.locator('body').innerText().catch(() => '')) || ''
  check('隐私页文案与实际行为一致', /使用日志始终记录|Usage logs always record/.test(ptext), ptext.slice(0, 200))
  // 页面用内部滚动容器（overflow-auto），scrollIntoView 对它无效；直接滚到底部露出隐私卡片
  await pp.evaluate(() => {
    for (const e of document.querySelectorAll('*')) {
      const cs = getComputedStyle(e)
      if (e.scrollHeight > e.clientHeight + 100 && /auto|scroll/.test(cs.overflowY)) e.scrollTop = e.scrollHeight
    }
  })
  await pp.waitForTimeout(800)
  await pp.screenshot({ path: OUT + '/profile-privacy.png', fullPage: true })

  check('无未预期的运行时错误', errors.length === 0, errors.slice(0, 3).join(' | '))
  fs.writeFileSync(OUT + '/results.json', JSON.stringify({ timestamp: new Date().toISOString(), pass, fail, results, consoleErrors: errors }, null, 2))
  await browser.close()
  console.log(`\n=== 结果：${pass} 通过 / ${fail} 失败 ===`)
  process.exit(fail === 0 ? 0 : 1)
}
main().catch((e) => { console.error(e); process.exit(1) })
