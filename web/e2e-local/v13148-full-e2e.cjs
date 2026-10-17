const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.E2E_BASE || 'http://127.0.0.1:18131'
const USER = process.env.E2E_USER || 'root'
const PASS = process.env.E2E_PASS || 'E2ePass!2026'
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.148')
const SK = process.env.E2E_SK || fs.readFileSync('C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/newapi-e2e/sk.txt', 'utf8').trim()

fs.mkdirSync(OUT, { recursive: true })
const redact = (v) => String(v).replace(/sk-[A-Za-z0-9]{10,}/g, "sk-REDACTED")
const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail: detail === undefined ? undefined : redact(detail) })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ` — ${detail}` : ''}`)
}

;(async () => {
  const browser = await chromium.launch({ args: ['--no-proxy-server', '--proxy-bypass-list=*'] })
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200)) })
  page.on('pageerror', (e) => consoleErrors.push(String(e).slice(0, 200)))

  try {
    // ---------- 登录 ----------
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.fill('input[name="username"], input#username', USER)
    await page.fill('input[name="password"], input#password', PASS)
    await page.getByRole('button', { name: /登录|Login|Sign in/i }).click()
    await page.waitForURL((u) => !u.pathname.includes('sign-in'), { timeout: 30000 })
    record('管理员登录', true, page.url())

    // ---------- Key 创建 + curl 示例 ----------
    await page.goto(`${BASE}/keys`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.getByRole('button', { name: /Create API Key|创建 API 密钥/i }).first().click({ timeout: 15000 })
    await page.fill('input[name="name"]', `e2e-ui-key-${Date.now()}`)
    await page.getByRole('button', { name: /Save changes|保存/i }).first().click({ timeout: 15000 })
    const curlBlock = page.getByTestId('created-key-curl')
    await curlBlock.waitFor({ state: 'visible', timeout: 30000 })
    const curlText = await curlBlock.innerText()
    record('创建 Key 后展示 curl', curlText.includes('/v1/chat/completions') && curlText.includes('Authorization: Bearer'), curlText.slice(0, 120))

    // ---------- MCP 付费工具 -> 待审批 ----------
    const approval = await page.evaluate(async ({ base, sk }) => {
      const init = await fetch(`${base}/mcp`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sk}` },
        body: JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', clientInfo: { name: 'browser-e2e' } } }),
      })
      const sid = init.headers.get('Mcp-Session-Id')
      let last = null
      // 本地 E2E 与构建/浏览器共用 CPU，性能门禁可能短暂 503；重试到拿到业务响应。
      for (let attempt = 0; attempt < 8; attempt++) {
        const r = await fetch(`${base}/mcp`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${sk}`, 'Mcp-Session-Id': sid },
          body: JSON.stringify({ jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'create_image', arguments: { prompt: 'e2e cat' } } }),
        })
        const text = await r.text()
        let body
        try { body = JSON.parse(text) } catch { body = { raw: text } }
        last = { status: r.status, sid, body }
        if (!text.includes('system_cpu_overloaded')) { break }
        await new Promise((resolve) => setTimeout(resolve, 2000))
      }
      return last
    }, { base: BASE, sk: SK })
    record('MCP 付费工具默认拒绝并生成审批', approval?.body?.error?.code === -32003 && approval?.body?.error?.data?.approval_id > 0, JSON.stringify(approval).slice(0, 300))

    // ---------- 动作审批页面 ----------
    await page.goto(`${BASE}/action-approvals`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(1500)
    const row = page.locator('tr', { hasText: 'create_image' }).first()
    await row.waitFor({ state: 'visible', timeout: 20000 })
    const approveBtn = row.getByRole('button', { name: /Approve|批准/i }).first()
    await approveBtn.click({ timeout: 15000 })
    await page.waitForTimeout(1200)
    const bodyText = await page.locator('body').innerText()
    record('动作审批页可用且可批准', /approved|已批准/i.test(bodyText), bodyText.slice(0, 160))
    await page.screenshot({ path: path.join(OUT, 'action-approvals.png'), fullPage: false })

    // ---------- 语言路径 / hreflang ----------
    await page.goto(`${BASE}/en/pricing`, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(1200)
    const lang = await page.evaluate(() => document.documentElement.lang)
    const canonical = await page.evaluate(() => document.querySelector('link[rel="canonical"]')?.getAttribute('href') || '')
    const hreflangCount = await page.evaluate(() => document.querySelectorAll('link[rel="alternate"][hreflang]').length)
    record('/en/pricing 语言与 canonical', lang === 'en' && canonical.includes('/en/pricing'), `lang=${lang} canonical=${canonical}`)
    record('/en/pricing hreflang', hreflangCount >= 3, `count=${hreflangCount}`)
    await page.screenshot({ path: path.join(OUT, 'en-pricing.png'), fullPage: false })

    // ---------- 公开页无致命 console 错误 ----------
    // 登录前的 /api/user/auth/refresh 401 是预期行为（尚无会话）。
    // 浏览器只把「Failed to load resource ... 401」写进 console，不含 URL，
    // 因此这里把纯网络资源错误排除；真正的 JS 异常仍由 pageerror 单独收集。
    const fatal = consoleErrors.filter(
      (e) => !/Failed to load resource/i.test(e) && !/favicon/i.test(e)
    )
    record('无致命 console 错误', fatal.length === 0, fatal.slice(0, 3).join(' | '))
  } catch (e) {
    record('E2E 执行', false, e.message)
  } finally {
    const ok = results.every((r) => r.ok)
    fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify({ ok, results, consoleErrors: consoleErrors.map(redact) }, null, 2))
    console.log(`E2E: ${results.filter((r) => r.ok).length}/${results.length} passed`)
    await browser.close()
    process.exit(ok ? 0 : 1)
  }
})()
