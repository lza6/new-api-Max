/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * 首页落地页改版视觉验证（本地，真实浏览器）
 *
 * 覆盖本轮改动：
 *   A. 首页整体渲染（editorial serif 标题、暖色 accent）
 *   B. 新增「请求流向」区块：默认 Chat 面板 + 延迟拆解条
 *   C. 切换端点到 Claude → 上游目标变为 Anthropic
 *   D. 深浅色两套主题下均可读
 *
 * 前置：`bun run build` 已产出 dist。脚本自带静态服务 + /api stub。
 * 输出：计划书/e2e-evidence/v1.3.77/ 截图 + results.json
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.77')
const PORT = 4173

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

function apiStub(url) {
  if (url.startsWith('/api/status')) {
    return {
      success: true,
      data: {
        system_name: 'new-api-Max',
        logo: '/logo.png',
        version: 'v1.3.77',
        docs_link: 'https://github.com/lza6/new-api-Max',
        user_agreement_enabled: false,
        privacy_policy_enabled: false,
      },
    }
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

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  // reducedMotion:'reduce' makes AnimateInView reveal content immediately
  // (no IntersectionObserver dependency), so a fullPage screenshot captures
  // every section deterministically. It is also a real accessibility path.
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'reduce',
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  const BASE = `http://127.0.0.1:${PORT}`

  // Site defaults to Simplified Chinese; pin English so the assertions below
  // can match stable source strings.
  await context.addInitScript(() => {
    window.localStorage.setItem('i18nextLng', 'en')
  })

  // AnimateInView hides content until IntersectionObserver fires; a fullPage
  // screenshot never scrolls, so below-fold sections stay at opacity-0. Walk
  // the page first to trigger every reveal, then jump back to the top.
  const revealAll = async () => {
    const height = await page.evaluate(() => document.body.scrollHeight)
    for (let y = 0; y < height; y += 600) {
      await page.evaluate((top) => window.scrollTo(0, top), y)
      await page.waitForTimeout(60)
    }
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(200)
  }

  try {
    await page.goto(BASE, { waitUntil: 'networkidle' })
    await revealAll()

    // A. Hero renders
    const heroHeading = await page
      .locator('h1')
      .first()
      .innerText()
      .catch(() => '')
    record('Hero 标题渲染', heroHeading.includes('Unified API Gateway'), heroHeading.slice(0, 60))

    // B. RequestFlow section renders with default Chat panel
    const flowHeading = await page
      .getByText('Every request takes the same path')
      .first()
      .isVisible()
      .catch(() => false)
    record('请求流向区块可见', flowHeading)

    const clientVisible = await page.getByText('Client', { exact: true }).first().isVisible()
    const gatewayVisible = await page.getByText('Gateway', { exact: true }).first().isVisible()
    record('流水线阶段 Client/Gateway 可见', clientVisible && gatewayVisible)

    const latencySegments = await page.getByText('Upstream compute').first().isVisible()
    record('延迟拆解含 Upstream compute', latencySegments)

    await page.getByText('Request Flow').first().scrollIntoViewIfNeeded()
    await shot(page, '01-home-light')

    // C. Switch to Claude tab
    await page.getByRole('tab', { name: 'Claude' }).click()
    await page.waitForTimeout(700)
    const anthropicVisible = await page.getByText('Anthropic').first().isVisible()
    record('切到 Claude 后上游显示 Anthropic', anthropicVisible)
    await shot(page, '02-requestflow-claude')

    // D. Dark theme
    await page.goto(BASE, { waitUntil: 'networkidle' })
    await page.evaluate(() => {
      document.documentElement.classList.add('dark')
    })
    await revealAll()
    await page.getByText('Request Flow').first().scrollIntoViewIfNeeded()
    await page.waitForTimeout(400)
    await shot(page, '03-requestflow-dark')
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(200)
    await shot(page, '04-home-dark')

    record('无控制台错误', consoleErrors.length === 0, consoleErrors.slice(0, 3).join(' | '))
  } catch (err) {
    record('脚本异常', false, String(err))
  } finally {
    fs.mkdirSync(OUT, { recursive: true })
    fs.writeFileSync(
      path.join(OUT, 'results.json'),
      JSON.stringify(results, null, 2)
    )
    await browser.close()
    server.close()
  }

  const failed = results.filter((r) => !r.ok)
  console.log(`\n${results.length - failed.length}/${results.length} passed`)
  process.exit(failed.length ? 1 : 0)
})()
