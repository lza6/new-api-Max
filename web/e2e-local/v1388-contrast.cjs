/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * 真实浏览器 a11y 对比度验证（本地）
 *
 * 为什么需要它：jsdom 不加载全局 CSS、不做样式计算，axe 的 color-contrast
 * 规则在 vitest 里落入 `incomplete`（无法判定），所以「vitest 断言 violations=[]」
 * 对对比度是假绿。真实对比度只能在真实浏览器里用 axe-core 跑。
 *
 * 覆盖关键公开页：home / pricing / sign-in。断言 axe 的 color-contrast 违规为 0。
 *
 * 前置：`bun run build` 已产出 dist。输出：计划书/e2e-evidence/v1.3.88/。
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const AXE_PATH = path.resolve(__dirname, '../node_modules/axe-core/axe.min.js')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.88')
const PORT = 4182

const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ` — ${detail}` : ''}`)
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
    return { success: true, data: { system_name: 'new-api-Max', logo: '/logo.png', version: 'v1.3.88', docs_link: 'https://x' } }
  }
  if (url.startsWith('/api/home_page_content') || url.startsWith('/api/notice')) {return { success: true, data: '' }}
  if (url.startsWith('/api/site/stats')) {return { success: false }}
  if (url.startsWith('/api/pricing')) {
    return {
      success: true,
      data: [{ id: 1, model_name: 'deepseek-v4.1-flash', quota_type: 0, model_ratio: 1, completion_ratio: 3, enable_groups: ['default'] }],
      vendors: [], group_ratio: { default: 1 }, usable_group: { default: { desc: 'Standard', ratio: 1 } }, supported_endpoint: {}, auto_groups: [],
    }
  }
  if (url.startsWith('/api/model/stats')) {return { success: true, data: { models: [] } }}
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

const AXE_SOURCE = fs.readFileSync(AXE_PATH, 'utf8')

async function runAxeContrast(page) {
  await page.addScriptTag({ content: AXE_SOURCE })
  // Run axe restricted to the contrast rule (structure/aria are covered by the
  // jsdom tests); jsdom cannot evaluate contrast, this is where it is verified.
  return page.evaluate(async () => {
    // eslint-disable-next-line no-undef
    const res = await window.axe.run(document, {
      runOnly: { type: 'rule', values: ['color-contrast'] },
    })
    return res.violations.map((v) => ({
      id: v.id,
      nodes: v.nodes.map((n) => (n.target || []).join(' ')).slice(0, 5),
    }))
  })
}

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  const BASE = `http://127.0.0.1:${PORT}`

  for (const [label, route] of [['home', '/'], ['pricing', '/pricing'], ['sign-in', '/sign-in']]) {
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
    await context.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
    const page = await context.newPage()
    try {
      await page.goto(`${BASE}${route}`, { waitUntil: 'networkidle' })
      // Wait until entrance animations settle. The hero uses staggered
      // `.landing-animate-fade-up` (0.6s + up to ~1s delay); mid-animation the
      // parent opacity dips and dilutes contrast, so a fixed wait is flaky.
      // Poll getAnimations() until none are running (bounded), then a short
      // settle. This keeps the contrast check on the final, painted state.
      await page
        .waitForFunction(
          () =>
            document.getAnimations().every((a) => a.playState !== 'running'),
          undefined,
          { timeout: 6000 }
        )
        .catch(() => {})
      await page.waitForTimeout(400)
      const violations = await runAxeContrast(page)
      record(
        `${label} 无 color-contrast 违规（真实浏览器）`,
        violations.length === 0,
        violations.length ? JSON.stringify(violations) : 'clean'
      )
    } catch (error) {
      record(`${label} 对比度检查异常`, false, String(error?.message ?? error))
    }
    await context.close()
  }

  fs.mkdirSync(OUT, { recursive: true })
  fs.writeFileSync(
    path.join(OUT, 'results-contrast.json'),
    JSON.stringify({ results }, null, 2)
  )
  const pass = results.filter((r) => r.ok).length
  console.log(`\nE2E(contrast): ${pass}/${results.length} passed`)
  await browser.close()
  server.close()
  process.exit(results.every((r) => r.ok) ? 0 : 1)
})()
