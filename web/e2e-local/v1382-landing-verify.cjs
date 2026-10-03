/* eslint-disable typescript/no-require-imports, no-console -- standalone Node E2E harness, mirrors sibling e2e-local scripts */
/**
 * 首页「强烈动效 + 3D + FAQ/SEO」验证（本地，真实浏览器）
 *
 * 覆盖本轮（v1.3.82）改动：
 *   A. Hero 仍正常渲染（逐词入场不破标题）
 *   B. WebGL 视觉：支持时出现 <canvas>，不支持/降级时回退 CSS 展示（视觉区永不为空）
 *   C. FAQ 区块可见 + 可展开（键盘可达）
 *   D. FAQPage JSON-LD 注入且与可见问答一致
 *   E. 无横向溢出（视差/3D 未撑破布局）
 *   F. 深浅色 + 移动端截图
 *   G. 无控制台错误
 *
 * 前置：`bun run build` 已产出 dist。脚本自带静态服务 + /api stub。
 * 输出：计划书/e2e-evidence/v1.3.82/ 截图 + results.json
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const { chromium } = require('playwright')

const DIST = path.resolve(__dirname, '../dist')
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.82')
const PORT = 4174

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
        version: 'v1.3.82',
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

function newPage(context) {
  return context.newPage()
}

async function revealAll(page) {
  const height = await page.evaluate(() => document.body.scrollHeight)
  for (let y = 0; y < height; y += 600) {
    await page.evaluate((top) => window.scrollTo(0, top), y)
    await page.waitForTimeout(60)
  }
  await page.evaluate(() => window.scrollTo(0, 0))
  await page.waitForTimeout(250)
}

;(async () => {
  const server = await serve()
  const browser = await chromium.launch()
  const BASE = `http://127.0.0.1:${PORT}`

  // ── Pass 1: normal motion, desktop — exercises the WebGL path if available.
  const normal = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'no-preference',
  })
  await normal.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const page = await newPage(normal)
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  try {
    await page.goto(BASE, { waitUntil: 'networkidle' })
    await revealAll(page)

    // A. Hero headline survives word-splitting animation.
    const heroHeading = await page.locator('h1').first().innerText().catch(() => '')
    record(
      'Hero 标题渲染（逐词入场未破文本）',
      heroHeading.includes('Unified API Gateway'),
      heroHeading.replace(/\s+/g, ' ').slice(0, 60)
    )

    // B. Visual area is never blank: a WebGL canvas OR the CSS showcase marker.
    await page.waitForTimeout(600)
    const hasCanvas = (await page.locator('canvas').count()) > 0
    const hasFallback =
      (await page.getByText('Smart Routing').count()) > 0 ||
      (await page.getByText('Transparent Billing').count()) > 0
    record(
      'Hero 视觉区非空（WebGL 或 CSS 回退）',
      hasCanvas || hasFallback,
      `canvas=${hasCanvas} fallback=${hasFallback}`
    )

    // C. FAQ section visible and keyboard-expandable.
    const faqHeading = await page
      .getByText('Frequently asked questions')
      .first()
      .isVisible()
      .catch(() => false)
    record('FAQ 区块可见', faqHeading)

    const question = page.getByText('What is an AI API gateway?').first()
    await question.scrollIntoViewIfNeeded()
    await page.waitForTimeout(300)
    await shot(page, '01-faq-collapsed')

    await question.click()
    await page.waitForTimeout(500)
    const answerVisible = await page
      .getByText('An AI API gateway is a single endpoint', { exact: false })
      .first()
      .isVisible()
      .catch(() => false)
    record('FAQ 展开后答案可见', answerVisible)
    await shot(page, '02-faq-expanded')

    // D. FAQPage JSON-LD injected and matching visible questions.
    const jsonLd = await page.evaluate(() => {
      const el = document.getElementById('landing-faq-jsonld')
      return el ? el.textContent : null
    })
    let ld = null
    try {
      ld = jsonLd ? JSON.parse(jsonLd) : null
    } catch {
      ld = null
    }
    record(
      'FAQPage JSON-LD 注入',
      Boolean(ld && ld['@type'] === 'FAQPage' && Array.isArray(ld.mainEntity)),
      ld ? `@type=${ld['@type']} count=${ld.mainEntity?.length}` : 'missing'
    )
    record(
      'JSON-LD 问题数与可见 FAQ 一致(6)',
      Boolean(ld && ld.mainEntity && ld.mainEntity.length === 6),
      ld?.mainEntity?.length
    )

    // E. No horizontal overflow (parallax/3D did not blow out the layout).
    const overflow = await page.evaluate(() => {
      const doc = document.documentElement
      return { scroll: doc.scrollWidth, client: doc.clientWidth }
    })
    record(
      '无横向溢出',
      overflow.scroll <= overflow.client + 2,
      `scroll=${overflow.scroll} client=${overflow.client}`
    )

    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(200)
    await shot(page, '03-home-light')

    // F. Dark theme
    await page.evaluate(() => document.documentElement.classList.add('dark'))
    await revealAll(page)
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(250)
    await shot(page, '04-home-dark')

    record('无控制台错误', consoleErrors.length === 0, consoleErrors.slice(0, 3).join(' | '))
  } catch (err) {
    record('脚本异常(normal)', false, err.stack || String(err))
  } finally {
    await normal.close()
  }

  // ── Pass 2: reduced motion — must fall back to the CSS showcase, no canvas.
  const reduced = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    reducedMotion: 'reduce',
  })
  await reduced.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const rpage = await newPage(reduced)
  try {
    await rpage.goto(BASE, { waitUntil: 'networkidle' })
    await revealAll(rpage)
    const rCanvas = await rpage.locator('canvas').count()
    const rFallback =
      (await rpage.getByText('Smart Routing').count()) > 0 ||
      (await rpage.getByText('Transparent Billing').count()) > 0
    record('reduced-motion 不启用 WebGL canvas', rCanvas === 0, `canvas=${rCanvas}`)
    record('reduced-motion 回退 CSS 展示非空', rFallback)
    await shot(rpage, '05-home-reduced-motion')
  } catch (err) {
    record('脚本异常(reduced)', false, String(err))
  } finally {
    await reduced.close()
  }

  // ── Pass 3: mobile width — no overflow, sections legible.
  const mobile = await browser.newContext({
    viewport: { width: 390, height: 844 },
    reducedMotion: 'reduce',
  })
  await mobile.addInitScript(() => window.localStorage.setItem('i18nextLng', 'en'))
  const mpage = await newPage(mobile)
  try {
    await mpage.goto(BASE, { waitUntil: 'networkidle' })
    await revealAll(mpage)
    const mOverflow = await mpage.evaluate(() => {
      const doc = document.documentElement
      return { scroll: doc.scrollWidth, client: doc.clientWidth }
    })
    record(
      '移动端无横向溢出',
      mOverflow.scroll <= mOverflow.client + 2,
      `scroll=${mOverflow.scroll} client=${mOverflow.client}`
    )
    await shot(mpage, '06-home-mobile')
  } catch (err) {
    record('脚本异常(mobile)', false, String(err))
  } finally {
    await mobile.close()
  }

  await browser.close()
  server.close()

  fs.mkdirSync(OUT, { recursive: true })
  fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify(results, null, 2))

  const failed = results.filter((r) => !r.ok)
  console.log(`\n${results.length - failed.length}/${results.length} passed`)
  process.exit(failed.length ? 1 : 0)
})()
