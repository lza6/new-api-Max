/**
 * 回归验证：边界接口返回**非字符串**（对象）时，首页不得整页 500。
 *
 * 复现的原始缺陷：
 *   - /api/home_page_content 返回对象 → marked() 抛 "input parameter is of type [object Object]"
 *   - /api/notice 返回对象 → "(c.data || '').trim is not a function"
 * 两处都被路由 errorComponent 接住，用户看到的是整页 500，而不是「没有自定义内容」。
 *
 * 用法：node e2e-local/fe-malformed-api.cjs
 * 退出码 0 = 页面正常渲染；1 = 仍然整页错误。
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')
const zlib = require('node:zlib')

const ROOT = path.join(__dirname, '..', 'dist')
const PORT = 4183
const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.woff2': 'font/woff2',
  '.png': 'image/png',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
}

const server = http.createServer((req, res) => {
  const p = decodeURIComponent((req.url || '/').split('?')[0])
  let f = path.join(ROOT, p)
  if (!f.startsWith(ROOT)) {return res.writeHead(403).end()}
  if (!fs.existsSync(f) || fs.statSync(f).isDirectory()) {f = path.join(ROOT, 'index.html')}
  const raw = fs.readFileSync(f)
  const ext = path.extname(f)
  if (/\bgzip\b/.test(req.headers['accept-encoding'] || '') && /\.(js|css|html|json|svg)$/.test(ext)) {
    res.writeHead(200, { 'Content-Type': MIME[ext], 'Content-Encoding': 'gzip', 'Cache-Control': 'no-cache' })
    return res.end(zlib.gzipSync(raw))
  }
  res.writeHead(200, { 'Content-Type': MIME[ext] || 'application/octet-stream', 'Cache-Control': 'no-cache' })
  res.end(raw)
})

;(async () => {
  const { chromium } = require('playwright')
  await new Promise((resolve) => server.listen(PORT, resolve))
  const browser = await chromium.launch()
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } })

  const pageErrors = []
  page.on('pageerror', (error) => pageErrors.push(String(error).slice(0, 200)))

  await page.route('**/api/**', (route) => {
    const url = route.request().url()
    // 故意返回与契约不符的类型
    if (url.includes('/api/home_page_content')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"unexpected":"object"}}' })
    }
    if (url.includes('/api/notice')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"unexpected":true}}' })
    }
    if (url.includes('/api/setup')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"status":true}}' })
    }
    if (url.includes('/api/status')) {
      return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{"system_name":"Baseline","logo":"","docs_link":""}}' })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: '{"success":true,"data":{}}' })
  })

  await page.goto(`http://127.0.0.1:${PORT}/`, { waitUntil: 'domcontentloaded', timeout: 120000 })
  await page.waitForSelector('#app-boot-loader', { state: 'detached', timeout: 120000 })
  await page.waitForTimeout(2000)

  const text = await page.evaluate(() => document.body.innerText.replace(/\s+/g, ' ').slice(0, 400))
  const isErrorPage = text.includes('糟糕') || /(^|\s)500(\s|$)/.test(text)
  const hasNav = text.includes('登录') || text.includes('Sign in') || text.includes('控制台')

  console.log(JSON.stringify({ isErrorPage, hasNav, pageErrors, textPreview: text.slice(0, 160) }, null, 2))
  await browser.close()
  server.close()
  if (isErrorPage || !hasNav) {
    console.error('FAIL: 畸形响应仍然导致整页错误')
    process.exit(1)
  }
  console.log('PASS: 畸形响应下首页正常渲染')
})().catch((error) => { console.error(error); server.close(); process.exit(1) })
