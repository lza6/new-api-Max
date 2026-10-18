const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.E2E_BASE || 'http://127.0.0.1:18131'
const USER = process.env.E2E_USER || 'root'
const PASS = process.env.E2E_PASS || 'E2ePass!2026'
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.150')
fs.mkdirSync(OUT, { recursive: true })
const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ` — ${detail}` : ''}`)
}

;(async () => {
  const browser = await chromium.launch({ args: ['--no-proxy-server', '--proxy-bypass-list=*'] })
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } })
  try {
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.fill('input[name="username"], input#username', USER)
    await page.fill('input[name="password"], input#password', PASS)
    await page.getByRole('button', { name: /登录|Login|Sign in/i }).click()
    await page.waitForURL((u) => !u.pathname.includes('sign-in'), { timeout: 30000 })

    const token = await page.evaluate(() => {
      const raw = localStorage.getItem('auth-storage') || localStorage.getItem('auth') || ''
      try { return JSON.parse(raw)?.state?.auth?.accessToken || '' } catch { return '' }
    })
    // 页面 localStorage 结构可能变化，退回到 API 登录拿 token。
    const apiToken = token || ''
    const call = async (url, opts = {}) => page.evaluate(async ({ base, t, u, o }) => {
      const r = await fetch(`${base}${u}`, { ...o, headers: { 'Content-Type': 'application/json', ...(t ? { Authorization: `Bearer ${t}` } : {}), ...(o.headers || {}) } })
      const text = await r.text()
      let body; try { body = JSON.parse(text) } catch { body = text }
      return { status: r.status, body }
    }, { base: BASE, t: apiToken, u: url, o: opts })

    // 用页面内 fetch 登录拿 access token（不依赖 localStorage 结构）。
    const login = await page.evaluate(async ({ base, u, p }) => {
      const r = await fetch(`${base}/api/user/login`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: u, password: p }) })
      return r.json()
    }, { base: BASE, u: USER, p: PASS })
    const adminToken = login?.data?.access_token || ''
    const adminCall = async (url, opts = {}) => page.evaluate(async ({ base, t, u, o }) => {
      const r = await fetch(`${base}${u}`, { ...o, headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${t}`, ...(o.headers || {}) } })
      const text = await r.text()
      let body; try { body = JSON.parse(text) } catch { body = text }
      return { status: r.status, body }
    }, { base: BASE, t: adminToken, u: url, o: opts })

    await adminCall('/api/option/payment_compliance', { method: 'POST', body: JSON.stringify({ confirmed: true }) })
    await adminCall('/api/option/feature-switches', { method: 'PUT', body: JSON.stringify({ key: 'SUBSCRIPTION_FIRST_PURCHASE_DISCOUNT', value: 'true' }) })

    const title = `E2E-First-${Date.now()}`
    const created = await adminCall('/api/subscription/admin/plans', {
      method: 'POST',
      body: JSON.stringify({
        plan: {
          title,
          subtitle: 'e2e',
          price_amount: 100,
          currency: 'CNY',
          duration_unit: 'month',
          duration_value: 1,
          custom_seconds: 0,
          quota_reset_period: 'never',
          quota_reset_custom_seconds: 0,
          enabled: true,
          sort_order: 999,
          allow_balance_pay: true,
          allow_wallet_overflow: true,
          max_purchase_per_user: 0,
          total_amount: 1000000,
          upgrade_group: '',
          downgrade_group: '',
          concurrency_limit: 0,
          rpm_limit: 0,
          models: '',
          first_purchase_discount_percent: 20,
        },
      }),
    })
    const planId = created?.body?.data?.id
    record('创建 20% 首购优惠套餐', !!planId, JSON.stringify(created?.body || {}))

    const before = await adminCall('/api/user/self')
    const quotaBefore = Number(before?.body?.data?.quota || 0)

    const plans = await adminCall('/api/subscription/plans')
    const plan = (plans?.body?.data || []).find((p) => p?.plan?.id === planId)
    record('计划列表返回首购折后价', Number(plan?.discounted_price) === 80, JSON.stringify(plan || {}))

    await page.goto(`${BASE}/pricing/plans`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(2000)
    const bodyText = await page.locator('body').innerText()
    record('订阅页展示首单优惠价', bodyText.includes('¥80.00') && /首单优惠|First purchase discount/i.test(bodyText), bodyText.slice(0, 200))
    await page.screenshot({ path: path.join(OUT, 'first-purchase-discount.png'), fullPage: false })

    const pay = await adminCall('/api/subscription/balance/pay', {
      method: 'POST',
      body: JSON.stringify({ plan_id: planId }),
    })
    record('余额支付按折后价扣款', pay?.body?.success === true, JSON.stringify(pay?.body || {}))

    const after = await adminCall('/api/user/self')
    const quotaAfter = Number(after?.body?.data?.quota || 0)
    const deducted = quotaBefore - quotaAfter
    record('扣款额度 = 80 × QuotaPerUnit', deducted === 80 * 500000, `deducted=${deducted}`)

    // 清理：停用测试套餐，关闭开关
    await adminCall(`/api/subscription/admin/plans/${planId}`, { method: 'PATCH', body: JSON.stringify({ enabled: false }) })
    await adminCall('/api/option/feature-switches', { method: 'PUT', body: JSON.stringify({ key: 'SUBSCRIPTION_FIRST_PURCHASE_DISCOUNT', value: 'false' }) })
  } catch (e) {
    record('E2E 执行', false, e.message)
  } finally {
    const ok = results.every((r) => r.ok)
    fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify({ ok, results }, null, 2))
    console.log(`E2E: ${results.filter((r) => r.ok).length}/${results.length} passed`)
    await browser.close()
    process.exit(ok ? 0 : 1)
  }
})()
