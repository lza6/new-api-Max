/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * Phase 1 新手转化漏斗验证（本地，真实浏览器）
 *
 * 覆盖本轮改动：
 *   A. 模型广场模型卡「去对话」按钮 → 跳 /playground?model=<name>
 *   B. Playground 读取 ?model= 深链并作为当前选中模型
 *   C. 建 Key 成功后抽屉停在「结果步」，明文可见 + 有复制按钮 + 有「前往对话」
 *   D. 首屏引导卡（OnboardingGuide）在登录后当次即出现（无需刷新）
 *   E. 无横向溢出 + 无控制台错误
 *
 * 前置：`bun run build` 已产出 dist。脚本自带静态服务 + /api stub（含认证）。
 * 输出：计划书/e2e-evidence/v1.3.85/ 截图 + results.json
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.85')
const PORT = 4176

const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ` — ${detail}` : ''}`)
}
const shot = async (page, name) => {
  fs.mkdirSync(OUT, { recursive: true })
  await page.screenshot({ path: path.join(OUT, `${name}.png`), fullPage: true })
}

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.png': 'image/png',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff',
}

const NOW = Math.floor(Date.now() / 1000)

function authBundle() {
  return {
    access_token: 'e2e-access-token',
    token_type: 'Bearer',
    access_expires_at: NOW + 3600,
    user: {
      id: 7,
      username: 'e2e-user',
      display_name: 'E2E User',
      role: 1,
      status: 1,
      quota: 0,
      used_quota: 0,
      request_count: 0,
      group: 'default',
    },
    session: {
      sid: 'e2e-sid',
      current: true,
      login_method: 'password',
      ip: '127.0.0.1',
      user_agent: 'e2e',
      created_at: NOW,
      last_active_at: NOW,
      expires_at: NOW + 3600,
    },
  }
}

function apiStub(url, method) {
  if (url.startsWith('/api/user/auth/refresh')) {
    return { success: true, data: authBundle() }
  }
  if (url.startsWith('/api/status')) {
    return {
      success: true,
      data: {
        system_name: 'new-api-Max',
        logo: '/logo.png',
        version: 'v1.3.85',
        docs_link: 'https://github.com/lza6/new-api-Max',
        user_agreement_enabled: false,
        privacy_policy_enabled: false,
      },
    }
  }
  if (url.startsWith('/api/user/self') && !url.includes('/groups')) {
    return { success: true, data: authBundle().user }
  }
  if (url.startsWith('/api/user/self/groups')) {
    return {
      success: true,
      data: { default: { desc: 'Standard access', ratio: 1 } },
    }
  }
  if (url.startsWith('/api/user/models')) {
    return {
      success: true,
      data: ['deepseek-v4.1-flash', 'gpt-4o-mini'],
    }
  }
  if (url.startsWith('/api/token/auto-groups')) {
    return { success: true, data: { groups: [], max_count: 3 } }
  }
  // Token list: empty so the onboarding guide has 0/3 progress.
  if (url.startsWith('/api/token/search') || url.startsWith('/api/token/?')) {
    return {
      success: true,
      data: { items: [], total: 0, page: 1, page_size: 10 },
    }
  }
  // Create token: return a key so the drawer shows the result step.
  if (url === '/api/token/' && method === 'POST') {
    return { success: true, data: { id: 42, key: 'e2e-plaintext-key-abc123' } }
  }
  if (url.startsWith('/api/token/')) {
    return { success: true, data: null }
  }
  // Pricing plaza: /api/pricing returns PricingData directly (not wrapped).
  if (url.startsWith('/api/pricing')) {
    return {
      success: true,
      data: [
        {
          id: 1,
          model_name: 'deepseek-v4.1-flash',
          quota_type: 0,
          model_ratio: 1,
          completion_ratio: 3,
          enable_groups: ['default'],
        },
      ],
      vendors: [],
      group_ratio: { default: 1 },
      usable_group: { default: { desc: 'Standard', ratio: 1 } },
      supported_endpoint: {},
      auto_groups: [],
    }
  }
  if (url.startsWith('/api/model/stats')) {
    return { success: true, data: { models: [] } }
  }
  if (url.startsWith('/api/home_page_content')) {
    return { success: true, data: '' }
  }
  if (url.startsWith('/api/site/stats')) {
    return { success: false }
  }
  if (url.startsWith('/api/notice')) {
    return { success: true, data: '' }
  }
  return { success: true, data: null }
}

function serve() {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const url = req.url.split('?')[0]
      if (url.startsWith('/api/')) {
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify(apiStub(req.url, req.method)))
        return
      }
      let file = path.join(DIST, url === '/' ? 'index.html' : url)
      if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) {
        file = path.join(DIST, 'index.html')
      }
      const ext = path.extname(file)
      res.writeHead(200, {
        'Content-Type': MIME[ext] || 'application/octet-stream',
      })
      fs.createReadStream(file).pipe(res)
    })
    server.listen(PORT, '127.0.0.1', () => resolve(server))
  })
}

async function gotoAndSettle(page, url) {
  await page.goto(url, { waitUntil: 'networkidle' })
  await page.waitForTimeout(800)
}

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  const BASE = `http://127.0.0.1:${PORT}`

  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  await context.addInitScript(() => {
    window.localStorage.setItem('i18nextLng', 'en')
    // Skip the onboarding dismissal so the guide is eligible to render.
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  try {
    // ── C + D: authenticated pages (keys list + onboarding guide).
    await gotoAndSettle(page, `${BASE}/keys`)

    // D. Onboarding guide appears on the first authenticated render (the
    // identity effect flips it in asynchronously, so wait for the element).
    const guide = page.getByTestId('onboarding-guide')
    const guideVisible = await guide
      .waitFor({ state: 'visible', timeout: 5000 })
      .then(() => true)
      .catch(() => false)
    record('登录后首屏引导卡当次出现（无需刷新）', guideVisible)
    if (guideVisible) {
      const progress = await guide.innerText().catch(() => '')
      record(
        '引导卡显示真实进度 0/3',
        progress.includes('0/3'),
        progress.replace(/\s+/g, ' ').slice(0, 60)
      )
    }
    await shot(page, '01-keys-with-guide')

    // C. Create a key → result step with plaintext + actions.
    await page.getByRole('button', { name: 'Create API Key' }).first().click()
    await page.waitForTimeout(700)
    const nameInput = page.locator('input#name, input[name="name"]').first()
    await nameInput.fill('e2e-key')
    await page.getByRole('button', { name: 'Save changes' }).click()
    await page.waitForTimeout(1200)

    const keyRow = page.getByTestId('created-key-row').first()
    const keyVisible = await keyRow.isVisible().catch(() => false)
    const keyText = keyVisible ? await keyRow.innerText().catch(() => '') : ''
    record(
      '建 Key 后抽屉停在结果步并展示明文',
      keyVisible && keyText.includes('sk-e2e-plaintext-key-abc123'),
      keyText.replace(/\s+/g, ' ').slice(0, 60)
    )
    const copyBtn = await page
      .getByRole('button', { name: 'Copy API key' })
      .isVisible()
      .catch(() => false)
    record('结果步有复制按钮', copyBtn)
    const toPlayground = await page
      .getByRole('button', { name: 'Go to Playground' })
      .isVisible()
      .catch(() => false)
    record('结果步有「前往对话」按钮', toPlayground)
    await shot(page, '02-key-created-result-step')

    // Result step's "Go to Playground" navigates to the playground.
    await page.getByRole('button', { name: 'Go to Playground' }).click()
    await page.waitForTimeout(1000)
    record('结果步跳转到 Playground', page.url().includes('/playground'))

    // ── A + B: model plaza → Chat → playground ?model= deep link.
    await gotoAndSettle(page, `${BASE}/pricing`)
    // Wait for the priced model card to render before probing the button.
    await page
      .getByText('deepseek-v4.1-flash')
      .first()
      .waitFor({ state: 'visible', timeout: 8000 })
      .catch(() => {})
    // The "Chat" label also exists in the endpoint-type filter, so scope the
    // locator to the model card (which contains the model name).
    const chatBtn = page
      .locator('[data-slot="card"]')
      .filter({ hasText: 'deepseek-v4.1-flash' })
      .getByRole('button', { name: 'Chat' })
      .first()
    const chatVisible = await chatBtn
      .waitFor({ state: 'visible', timeout: 5000 })
      .then(() => true)
      .catch(() => false)
    record('模型卡出现「Chat（去对话）」按钮', chatVisible)
    await shot(page, '03-pricing-chat-button')

    if (chatVisible) {
      await chatBtn.click()
      await page.waitForURL(/\/playground/, { timeout: 5000 }).catch(() => {})
      await page.waitForTimeout(500)
      const urlModel = new URL(page.url()).searchParams.get('model')
      record(
        '点击 Chat 跳转 /playground?model=<name>',
        page.url().includes('/playground') && !!urlModel,
        `url=${page.url().slice(0, 90)}`
      )
    }

    // B. Deep link makes the seeded model the current selection. The selected
    // model renders inside the model/group combobox in the input bar.
    await gotoAndSettle(page, `${BASE}/playground?model=deepseek-v4.1-flash`)
    await page
      .getByRole('combobox')
      .first()
      .waitFor({ state: 'visible', timeout: 8000 })
      .catch(() => {})
    await page.waitForTimeout(500)
    const selectorText = await page
      .getByRole('combobox')
      .first()
      .innerText()
      .catch(() => '')
    record(
      'Playground 接受 ?model= 深链并选中该模型',
      selectorText.includes('deepseek-v4.1-flash'),
      selectorText.replace(/\s+/g, ' ').slice(0, 60)
    )
    await shot(page, '04-playground-deeplink')

    // ── E: no horizontal overflow.
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth
    )
    record('无横向溢出', overflow <= 1, `overflowPx=${overflow}`)

    record('无控制台错误', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
  } catch (error) {
    record('E2E 执行异常', false, String(error && error.message ? error.message : error))
  }

  fs.mkdirSync(OUT, { recursive: true })
  fs.writeFileSync(
    path.join(OUT, 'results.json'),
    JSON.stringify({ results, consoleErrors }, null, 2)
  )
  const pass = results.filter((r) => r.ok).length
  console.log(`\nE2E: ${pass}/${results.length} passed`)
  await browser.close()
  server.close()
  process.exit(results.every((r) => r.ok) ? 0 : 1)
})()
