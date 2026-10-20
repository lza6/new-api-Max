/**
 * 首屏 vitals 测量（LCP 用 buffered observer 在 load 后补装，规避 getEntriesByType 不暴露 LCP 的限制）
 * 用法：node e2e-local/fe-vitals.cjs [--throttle] [out.json]
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const ROOT = path.join(__dirname, '..', 'dist')
const PORT = 4179
const THROTTLE = process.argv.includes('--throttle')
const OUT = process.argv.find((a) => a.endsWith('.json')) || (THROTTLE ? 'fe-vitals-throttled.json' : 'fe-vitals-local.json')
const MIME = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.woff2': 'font/woff2', '.png': 'image/png', '.svg': 'image/svg+xml', '.ico': 'image/x-icon' }
const zlib = require('node:zlib')
// 构建产物在进程启动时**一次性预压缩**。若在请求回调里同步 gzip，Node 单线程会
// 被大文件压缩阻塞，导致小 chunk 出现 1.8s 的假性慢请求（实测踩过）。
const gzipped = new Map()
function precompress(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      precompress(full)
      continue
    }
    if (/\.(js|css|html|json|svg)$/.test(entry.name)) {
      gzipped.set(full, zlib.gzipSync(fs.readFileSync(full)))
    }
  }
}
precompress(ROOT)
const server = http.createServer((req, res) => {
  const p = decodeURIComponent((req.url || '/').split('?')[0])
  let f = path.join(ROOT, p)
  if (!f.startsWith(ROOT)) return res.writeHead(403).end()
  if (!fs.existsSync(f) || fs.statSync(f).isDirectory()) f = path.join(ROOT, 'index.html')
  const ext = path.extname(f)
  const acceptsGzip = /\bgzip\b/.test(req.headers['accept-encoding'] || '')
  const gz = gzipped.get(f)
  if (acceptsGzip && gz) {
    res.writeHead(200, {
      'Content-Type': MIME[ext] || 'application/octet-stream',
      'Content-Encoding': 'gzip',
      'Cache-Control': 'no-cache',
    })
    return res.end(gz)
  }
  res.writeHead(200, { 'Content-Type': MIME[ext] || 'application/octet-stream', 'Cache-Control': 'no-cache' })
  res.end(fs.readFileSync(f))
})

;(async () => {
  const { chromium } = require('playwright')
  await new Promise((r) => server.listen(PORT, r))
  const browser = await chromium.launch()
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 3,
    isMobile: true,
    hasTouch: true,
  })
  const page = await context.newPage()
  await page.route('**/api/**', (route) => {
    const url = route.request().url()
    if (url.includes('/api/notice')) return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":""}' })
    if (url.includes('/api/setup')) {
      // 已初始化：否则 __root.beforeLoad 会把页面重定向到 /setup
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"status":true}}' })
    }
    if (url.includes('/api/home_page_content')) {
      // 后端契约是字符串（markdown）；返回对象会让 marked() 抛错并触发整页 500
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: '' }),
      })
    }
    const body = url.includes('/api/status')
      ? '{"success":true,"data":{"system_name":"Baseline","logo":"","docs_link":""}}'
      : '{"success":true,"data":{}}'
    return route.fulfill({ status: 200, contentType: 'application/json', body })
  })
  if (THROTTLE) {
    const cdp = await context.newCDPSession(page)
    await cdp.send('Network.enable')
    await cdp.send('Network.emulateNetworkConditions', {
      offline: false,
      latency: 150,
      downloadThroughput: (1.6 * 1024 * 1024) / 8,
      uploadThroughput: (750 * 1024) / 8,
    })
    await cdp.send('Emulation.setCPUThrottlingRate', { rate: 4 })
  }

  const started = Date.now()
  await page.goto(`http://127.0.0.1:${PORT}/`, { waitUntil: 'domcontentloaded', timeout: 180000 })
  await page.waitForSelector('#app-boot-loader', { state: 'detached', timeout: 180000 })
  const mountedMs = Date.now() - started
  await page.waitForTimeout(1500)

  // LCP 必须在 load 后补装：buffered 会回放已发生的条目
  const vitals = await page.evaluate(async () => {
    const acc = { lcp: 0, lcpElement: '', cls: 0, lcpCandidates: [] }
    const lcpOb = new PerformanceObserver((list) => {
      for (const e of list.getEntries()) {
        acc.lcpCandidates.push({ t: Math.round(e.startTime), size: e.size, tag: e.element?.tagName || '', text: (e.element?.textContent || '').slice(0, 30) })
        if (e.startTime > acc.lcp) {
          acc.lcp = Math.round(e.startTime)
          acc.lcpElement = e.element?.tagName || ''
        }
      }
    })
    lcpOb.observe({ type: 'largest-contentful-paint', buffered: true })
    const clsOb = new PerformanceObserver((list) => {
      for (const e of list.getEntries()) if (!e.hadRecentInput) acc.cls += e.value
    })
    clsOb.observe({ type: 'layout-shift', buffered: true })
    await new Promise((r) => setTimeout(r, 800))
    const paint = performance.getEntriesByType('paint')
    const fcp = paint.find((p) => p.name === 'first-contentful-paint')
    const nav = performance.getEntriesByType('navigation')[0]
    return {
      fcp: fcp ? Math.round(fcp.startTime) : 0,
      lcp: acc.lcp,
      lcpElement: acc.lcpElement,
      lcpCandidates: acc.lcpCandidates,
      cls: Math.round(acc.cls * 1000) / 1000,
      domContentLoaded: nav ? Math.round(nav.domContentLoadedEventEnd) : 0,
      load: nav ? Math.round(nav.loadEventEnd) : 0,
      transferKB: Math.round(
        performance.getEntriesByType('resource')
          .filter((r) => r.name.includes('/static/'))
          .reduce((s, r) => s + (r.transferSize || 0), 0) / 1024
      ),
    }
  })

  const resources = await page.evaluate(() =>
    performance.getEntriesByType('resource')
      .filter((r) => r.name.includes('/static/'))
      .map((r) => ({
        name: r.name.split('/').pop(),
        start: Math.round(r.startTime),
        end: Math.round(r.startTime + r.duration),
        transferKB: Math.round((r.transferSize || 0) / 1024),
      }))
      .sort((a, b) => a.start - b.start)
  )

  const report = {
    measuredAt: new Date().toISOString(),
    mode: THROTTLE ? '1.6Mbps/150ms + 4x CPU (Lighthouse slow-4G profile)' : 'local, no throttle',
    mountedMs,
    vitals,
    resources,
  }
  fs.writeFileSync(path.join(__dirname, OUT), JSON.stringify(report, null, 2))
  await page.screenshot({ path: path.join(__dirname, THROTTLE ? 'fe-vitals-throttled.png' : 'fe-vitals-local.png') }).catch(() => {})
  await browser.close()
  server.close()
  console.log(JSON.stringify({ mode: report.mode, mountedMs, vitals, resourceCount: resources.length }, null, 2))
})().catch((e) => { console.error(e); server.close(); process.exit(1) })
