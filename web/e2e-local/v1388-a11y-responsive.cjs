/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * a11y + 响应式基线验证（本地，真实浏览器，Phase 2）
 *
 * 覆盖：
 *   A. 公开页 SkipToMain（#content）真实生效——Tab 首个焦点 + Enter 后焦点落到主内容
 *   B. 登录页 SkipToMain（#content）生效
 *   C. viewport-fit=cover 存在于 meta viewport
 *   D. 6 断点（320/375/768/1024/1440/1920）无横向溢出
 *   E. 暗色主题渲染 + 无控制台错误
 *
 * 前置：`bun run build` 已产出 dist。脚本自带静态服务 + /api stub。
 * 输出：计划书/e2e-evidence/v1.3.88/ 截图 + results.json
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.88')
const PORT = 4178

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

// Read the built index.html so we assert the REAL shipped markup (viewport meta).
function servedIndexHtml() {
  const raw = fs.readFileSync(path.join(DIST, 'index.html'), 'utf8')
  return raw
}

function apiStub(url) {
  if (url.startsWith('/api/status')) {
    return {
      success: true,
      data: {
        system_name: 'new-api-Max',
        logo: '/logo.png',
        version: 'v1.3.88',
        docs_link: 'https://github.com/lza6/new-api-Max',
        user_agreement_enabled: false,
        privacy_policy_enabled: false,
      },
    }
  }
  if (url.startsWith('/api/home_page_content')) {return { success: true, data: '' }}
  if (url.startsWith('/api/site/stats')) {return { success: false }}
  if (url.startsWith('/api/notice')) {return { success: true, data: '' }}
  return { success: true, data: null }
}

function serve() {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      const url = req.url.split('?')[0]
      if (url.startsWith('/api/')) {
        res.writeHead(200, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify(apiStub(url)))
        return
      }
      let file = path.join(DIST, url === '/' ? 'index.html' : url)
      if (!fs.existsSync(file) || fs.statSync(file).isDirectory()) {
        file = path.join(DIST, 'index.html')
      }
      const ext = path.extname(file)
      res.writeHead(200, { 'Content-Type': MIME[ext] || 'application/octet-stream' })
      fs.createReadStream(file).pipe(res)
    })
    server.listen(PORT, '127.0.0.1', () => resolve(server))
  })
}

// Press Tab from a cold state and report the resulting active element.
// A real keyboard user tabs from the top of the document; the first tabbable
// element in DOM order is what the skip link must be. Do not blur() first —
// that can leave focus in a state where the first Tab is swallowed.
async function firstTabTarget(page) {
  // Wait until the skip link is actually in the DOM (SPA hydration on the
  // heavy landing page can lag behind `networkidle`), then Tab from the top.
  await page
    .locator('a[href="#content"]')
    .first()
    .waitFor({ state: 'attached', timeout: 8000 })
    .catch(() => {})
  await page.waitForTimeout(150)
  await page.keyboard.press('Tab')
  await page.waitForTimeout(150)
  return page.evaluate(() => {
    const el = document.activeElement
    return {
      tag: el?.tagName,
      text: (el?.textContent || '').trim().slice(0, 40),
      href: el?.getAttribute?.('href'),
    }
  })
}

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  const BASE = `http://127.0.0.1:${PORT}`

  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  })
  await context.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  try {
    // C. viewport-fit=cover present in the SHIPPED index.html.
    const html = servedIndexHtml()
    record(
      'index.html 含 viewport-fit=cover',
      /viewport-fit=cover/.test(html),
      (html.match(/<meta name="viewport"[^>]*>/) || [''])[0].slice(0, 90)
    )

    // A. Public page: SkipToMain is the first tab stop and jumps to #content.
    await page.goto(BASE, { waitUntil: 'networkidle' })
    const first = await firstTabTarget(page)
    record(
      '公开页首个 Tab 焦点为 Skip to Main',
      first.tag === 'A' && /Skip to Main/i.test(first.text) && first.href === '#content',
      JSON.stringify(first)
    )
    // Activate it and confirm focus lands on the #content target.
    await page.keyboard.press('Enter')
    await page.waitForTimeout(250)
    const afterSkip = await page.evaluate(() => {
      const el = document.activeElement
      return { id: el?.id, tag: el?.tagName }
    })
    record(
      'Enter 后焦点落到 #content 目标',
      afterSkip.id === 'content',
      JSON.stringify(afterSkip)
    )

    // Verify a #content element actually exists on the public page.
    const hasContent = await page.locator('#content').count()
    record('公开页存在 #content 目标元素', hasContent > 0, `count=${hasContent}`)

    // E part 1: dark theme renders.
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.waitForTimeout(300)
    await shot(page, '01-home-dark')

    // B. Sign-in page (public) also has the skip link working.
    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(600)
    const signInContent = await page.locator('#content').count()
    const signInFirst = await firstTabTarget(page)
    record(
      '登录页 Skip to Main 生效且有 #content',
      signInContent > 0 && /Skip to Main/i.test(signInFirst.text),
      `contentCount=${signInContent} first=${JSON.stringify(signInFirst)}`
    )

    // D. No horizontal overflow across 6 breakpoints.
    const widths = [320, 375, 768, 1024, 1440, 1920]
    let worst = { w: 0, overflow: 0 }
    for (const w of widths) {
      await page.setViewportSize({ width: w, height: 900 })
      await page.goto(BASE, { waitUntil: 'networkidle' })
      await page.waitForTimeout(500)
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth
      )
      if (overflow > worst.overflow) {worst = { w, overflow }}
    }
    record(
      '6 断点无横向溢出',
      worst.overflow <= 1,
      `worst w=${worst.w} overflow=${worst.overflow}px`
    )

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
