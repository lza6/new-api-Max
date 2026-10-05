/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * DataTable 错误态端到端验证（本地，真实浏览器）
 *
 * 覆盖：列表查询失败时，真实渲染「错误 + 重试」而非「暂无数据」空态。
 * 用 /api/token/ 返回 success:false 模拟查询失败，检查密钥页出现错误态与 Retry。
 *
 * 前置：`bun run build` 已产出 dist。
 * 输出：计划书/e2e-evidence/v1.3.88/ 截图 + results.json
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.88')
const PORT = 4180
const NOW = Math.floor(Date.now() / 1000)

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

const bundle = () => ({
  access_token: 'e2e',
  token_type: 'Bearer',
  access_expires_at: NOW + 3600,
  user: { id: 7, username: 'u', role: 1, status: 1, quota: 0, used_quota: 0, request_count: 0, group: 'default' },
  session: { sid: 's', current: true, login_method: 'password', ip: '1', user_agent: 'e', created_at: NOW, last_active_at: NOW, expires_at: NOW + 3600 },
})

// Toggle: when true, the token list endpoint FAILS (success:false) so the
// table must render its error state instead of an empty list.
let failTokens = false

function apiStub(fullUrl) {
  const url = fullUrl.split('?')[0] // path only, for matching
  if (url.startsWith('/api/user/auth/refresh')) {return { success: true, data: bundle() }}
  if (url.startsWith('/api/status')) {
    return { success: true, data: { system_name: 'new-api-Max', logo: '/logo.png', version: 'v1.3.88', docs_link: 'https://x' } }
  }
  if (url.startsWith('/api/user/self/groups')) {return { success: true, data: { default: { desc: 'Standard', ratio: 1 } } }}
  if (url.startsWith('/api/user/self')) {return { success: true, data: bundle().user }}
  if (url.startsWith('/api/user/models')) {return { success: true, data: ['deepseek-v4.1-flash'] }}
  if (url.startsWith('/api/token/auto-groups')) {return { success: true, data: { groups: [], max_count: 3 } }}
  // The list/search endpoint under test (path-only match).
  if (url === '/api/token/' || url.startsWith('/api/token/search')) {
    if (failTokens) {return { success: false, message: 'simulated backend failure' }}
    return { success: true, data: { items: [], total: 0, page: 1, page_size: 10 } }
  }
  if (url.startsWith('/api/notice') || url.startsWith('/api/home_page_content')) {return { success: true, data: '' }}
  return { success: true, data: null }
}

function serve() {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const url = req.url.split('?')[0]
      if (url.startsWith('/api/')) {
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify(apiStub(req.url)))
        return
      }
      let file = path.join(DIST, url === '/' ? 'index.html' : url)
      if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) {file = path.join(DIST, 'index.html')}
      res.writeHead(200, { 'Content-Type': MIME[path.extname(file)] || 'application/octet-stream' })
      fs.createReadStream(file).pipe(res)
    })
    server.listen(PORT, '127.0.0.1', () => resolve(server))
  })
}

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  const BASE = `http://127.0.0.1:${PORT}`
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  await context.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') {consoleErrors.push(m.text())} })

  try {
    // 1. Sanity: with a healthy (empty) list, the table shows the empty state,
    //    NOT the error state.
    failTokens = false
    await page.goto(`${BASE}/keys`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(1200)
    const emptyShown = await page.getByText('No API Keys Found').first().isVisible().catch(() => false)
    const errWhenHealthy = await page.getByRole('button', { name: 'Retry' }).first().isVisible().catch(() => false)
    record('列表正常（空）时不显示错误态', emptyShown && !errWhenHealthy, `empty=${emptyShown} retry=${errWhenHealthy}`)

    // 2. Now make the list endpoint fail and reload → error state must appear.
    //    Production retry policy allows up to 4 attempts with exponential
    //    backoff (~15s total) before isError flips, so wait generously.
    failTokens = true
    await page.goto(`${BASE}/keys`, { waitUntil: 'networkidle' })
    const retry = page.getByRole('button', { name: 'Retry' }).first()
    const retryAppeared = await retry
      .waitFor({ state: 'visible', timeout: 30000 })
      .then(() => true)
      .catch(() => false)
    const emptyWhenError = await page
      .getByText('No API Keys Found')
      .first()
      .isVisible()
      .catch(() => false)
    record(
      '查询失败时渲染错误态 + 重试按钮（非空态）',
      retryAppeared && !emptyWhenError,
      `retry=${retryAppeared} empty=${emptyWhenError}`
    )
    await shot(page, '05-keys-error-state')

    // 3. Retry works: flip back to healthy, click Retry, error clears.
    failTokens = false
    if (retryAppeared) {
      await retry.click()
      await page.waitForTimeout(2000)
      const errGone = !(await page
        .getByRole('button', { name: 'Retry' })
        .first()
        .isVisible()
        .catch(() => false))
      record('点击重试后错误态消失（重取成功）', errGone)
    }

    record('无控制台错误', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
  } catch (error) {
    record('E2E 执行异常', false, String(error && error.message ? error.message : error))
  }

  fs.mkdirSync(OUT, { recursive: true })
  const resultsPath = path.join(OUT, 'results-error-state.json')
  fs.writeFileSync(resultsPath, JSON.stringify({ results, consoleErrors }, null, 2))
  const pass = results.filter((r) => r.ok).length
  console.log(`\nE2E(error-state): ${pass}/${results.length} passed`)
  await browser.close()
  server.close()
  process.exit(results.every((r) => r.ok) ? 0 : 1)
})()
