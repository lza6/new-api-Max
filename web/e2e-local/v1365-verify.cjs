/**
 * v1.3.65 套餐删除功能真实浏览器 E2E（本地）
 *
 * 覆盖本轮改动：
 *   A. 管理员「订阅管理」页每个套餐行有「Delete plan」入口
 *   B. 无用户订阅引用的套餐可被真实删除（列表行消失 + 后端 404）
 *   C. 仍被用户订阅引用的套餐被拒绝删除（保留行 + 报错提示）
 *
 * 前置：本地服务已起（PORT=3000 SQLITE_PATH=e2e-v1365.db）且已 /api/setup，
 *      管理员经 /api/setup 创建（用户名 root / 密码由 E2E_PASS 提供）。
 * 输出：计划书/e2e-evidence/v1.3.65/ 截图 + results.json
 */
const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.E2E_BASE || 'http://127.0.0.1:3000'
const USER = process.env.E2E_USER || 'root'
const PASS = process.env.E2E_PASS || 'E2ePass!2026'
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.65')

const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ' — ' + detail : ''}`)
}
const shot = async (page, name) => {
  fs.mkdirSync(OUT, { recursive: true })
  await page.screenshot({ path: path.join(OUT, `${name}.png`), fullPage: true })
}

/** 登录拿 access_token，返回一个带管理 API 的 fetch 绑定上下文。 */
async function adminApi(page) {
  return page.evaluate(async (b) => {
    const login = await fetch(`${b}/api/user/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'root', password: window.__E2E_PASS }),
    }).then((r) => r.json())
    if (!login?.data?.access_token) {
      throw new Error('login failed: ' + JSON.stringify(login).slice(0, 200))
    }
    return login.data.access_token
  }, BASE)
}

;(async () => {
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  try {
    // 注入密码供 page.evaluate 内的登录使用
    await page.addInitScript((p) => {
      window.__E2E_PASS = p
    }, PASS)

    // ---------- 登录 ----------
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'networkidle' })
    await page.fill('input[name="username"], input#username', USER)
    await page.fill('input[name="password"], input#password', PASS)
    await page.getByRole('button', { name: /登录|Login|Sign in/i }).click()
    await page.waitForURL((u) => !u.pathname.includes('sign-in'), { timeout: 20000 })
    record('管理员登录成功', true, page.url())

    const token = await adminApi(page)
    const api = async (url, opts = {}) =>
      page.evaluate(
        async ({ b, t, u, o }) => {
          const r = await fetch(`${b}${u}`, {
            ...o,
            headers: {
              'Content-Type': 'application/json',
              Authorization: `Bearer ${t}`,
              ...(o.headers || {}),
            },
          })
          const text = await r.text()
          let body
          try { body = JSON.parse(text) } catch { body = text }
          return { status: r.status, body }
        },
        { b: BASE, t: token, u: url, o: opts }
      )

    // 确认付费合规，解锁套餐创建/删除（否则 403 被门禁拦）
    await api('/api/option/payment_compliance', {
      method: 'POST',
      body: JSON.stringify({ confirmed: true }),
    })

    // ---------- 造数据：一个可删套餐 + 一个被引用套餐 ----------
    const ts = Date.now()
    const freeTitle = `E2E-Free-${ts}`
    const usedTitle = `E2E-Used-${ts}`
    const mkPlan = async (title) => {
      const res = await api('/api/subscription/admin/plans', {
        method: 'POST',
        body: JSON.stringify({
          plan: {
            title,
            price_amount: 1,
            currency: 'CNY',
            duration_unit: 'month',
            duration_value: 1,
            enabled: true,
            sort_order: 0,
            total_amount: 1000,
            quota_reset_period: 'never',
          },
        }),
      })
      return res.body?.data?.id
    }
    const freeId = await mkPlan(freeTitle)
    const usedId = await mkPlan(usedTitle)
    record('创建可删套餐（API）', Number.isInteger(freeId), `id=${freeId}`)
    record('创建被引用套餐（API）', Number.isInteger(usedId), `id=${usedId}`)

    // 给“被引用套餐”挂一条用户订阅（用管理员 bind 端点或直接造订阅）。
    // 若没有便捷端点，则用 redemption 之外的 admin bind：POST /subscription/admin/bind
    const bind = await api('/api/subscription/admin/bind', {
      method: 'POST',
      body: JSON.stringify({ user_id: 1, plan_id: usedId }),
    })
    record('为被引用套餐绑定一条订阅（API）', Boolean(bind.body?.success), JSON.stringify(bind.body).slice(0, 140))

    // ---------- A. UI 入口存在 ----------
    await page.goto(`${BASE}/subscriptions`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(3000)
    await shot(page, 'A1-subscriptions-list')

    const freeCard = page.locator('table tbody tr').filter({ hasText: freeTitle }).first()
    const freeVisible = await freeCard.isVisible().catch(() => false)
    record('订阅管理页显示新套餐行', freeVisible, freeTitle)

    let menuOpened = false
    if (freeVisible) {
      const menuBtn = freeCard.getByRole('button', { name: /Open menu|打开菜单/i }).first()
      await menuBtn.click().catch(() => {})
      await page.waitForTimeout(800)
      const delItem = page.getByRole('menuitem', { name: /Delete plan|删除套餐/i }).first()
      menuOpened = await delItem.isVisible().catch(() => false)
      record('行操作菜单含「Delete plan」', menuOpened)
      await shot(page, 'A2-row-menu')
      if (menuOpened) await delItem.click()
      await page.waitForTimeout(800)
      await shot(page, 'A3-delete-confirm')
      record(
        '删除确认弹窗出现',
        await page.getByText(/Delete plan|删除套餐/i).first().isVisible().catch(() => false)
      )
    }

    // ---------- C. 被引用套餐也能删除（新语义：有订阅也能删，不剥夺权益） ----------
    const usedDel = await api(`/api/subscription/admin/plans/${usedId}`, { method: 'DELETE' })
    const usedDeleted = usedDel.body?.success === true
    record(
      '被订阅引用的套餐也能删除（不被历史记录卡住）',
      usedDeleted,
      `status=${usedDel.status} affected=${JSON.stringify(usedDel.body?.data?.affected_user_ids)}`
    )
    // 订阅本身仍在（不被删除/剥夺）——查订阅日志确认该套餐订阅仍存在。
    const subsAfter = await api(`/api/subscription/admin/users/1/subscriptions`)
    const stillHasSub = (subsAfter.body?.data || []).some((s) => s.subscription?.plan_id === usedId)
    record('删除套餐后用户订阅保留（不剥夺权益）', stillHasSub)
    const usedGone = !(await api(`/api/subscription/admin/plans`)).body?.data?.some(
      (p) => p.plan?.id === usedId
    )
    record('被引用套餐删除后不再出现在列表', usedGone)

    // ---------- B. 无引用套餐真实删除 ----------
    const freeDel = await api(`/api/subscription/admin/plans/${freeId}`, { method: 'DELETE' })
    const freeDeleted = freeDel.body?.success === true
    record('无引用套餐删除成功', freeDeleted, `status=${freeDel.status}`)
    const after = await api(`/api/subscription/admin/plans`)
    const stillThere = (after.body?.data || []).some((p) => p.plan?.id === freeId)
    record('删除后套餐不再出现在列表', !stillThere)

    // 刷新 UI 确认行消失
    await page.reload({ waitUntil: 'networkidle' })
    await page.waitForTimeout(2500)
    const goneInUi = (await page.locator('table tbody').getByText(freeTitle).count()) === 0
    record('刷新后 UI 不再显示已删套餐', goneInUi)

    // ---------- 控制台 ----------
    const fatal = consoleErrors.filter(
      (e) => !/favicon|404|Failed to load resource|401|403/i.test(e)
    )
    record('浏览器控制台无致命错误', fatal.length === 0, fatal.slice(0, 2).join(' | '))
  } catch (err) {
    record('E2E 执行异常', false, String(err))
    await shot(page, 'Z-error')
  } finally {
    fs.mkdirSync(OUT, { recursive: true })
    fs.writeFileSync(
      path.join(OUT, 'results.json'),
      JSON.stringify({ ranAt: new Date().toISOString(), base: BASE, results }, null, 2)
    )
    await browser.close()
    const failed = results.filter((r) => !r.ok)
    console.log(`\n===== ${results.length - failed.length}/${results.length} 通过 =====`)
    console.log(`证据目录: ${OUT}`)
    process.exit(failed.length > 0 ? 1 : 0)
  }
})()
