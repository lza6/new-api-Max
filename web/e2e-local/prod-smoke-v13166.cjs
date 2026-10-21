/**
 * 生产真实浏览器冒烟（v1.3.166 部署后）
 *
 * 按 memory `new-api-prod-ui-e2e-tunnel` 的手法：
 *  - SSH 隧道指向 443（Secure cookie 才会下发）
 *  - Chromium `--host-resolver-rules` 把域名映射到 127.0.0.1
 *  - ignoreHTTPSErrors + 隧道端口 9600
 *
 * 只验证公开页（无管理员凭证，登录态路径不在本脚本范围）。
 * 断言：HTTP 200、h1 有文本、无 console error、320px 无横向溢出。
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const PATHS = ['/', '/pricing', '/status', '/sign-in']

async function main() {
  const browser = await chromium.launch({
    args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'],
  })
  const results = []
  for (const width of [390, 1440]) {
    const context = await browser.newContext({
      baseURL: BASE,
      ignoreHTTPSErrors: true,
      viewport: { width, height: width === 390 ? 844 : 900 },
      deviceScaleFactor: 1,
    })
    for (const path of PATHS) {
      const page = await context.newPage()
      const errors = []
      page.on('pageerror', (e) => errors.push(String(e).slice(0, 160)))
      page.on('console', (m) => {
        if (m.type() === 'error') errors.push('console: ' + m.text().slice(0, 160))
      })
      let status = 0
      try {
        const response = await page.goto(BASE + path, {
          waitUntil: 'domcontentloaded',
          timeout: 45000,
        })
        status = response ? response.status() : 0
        // 冷启动时路由 chunk 可能 >2.5s；等 #root 真有子节点再判定
        await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 20000 })
        await page.waitForTimeout(1200)
      } catch (error) {
        errors.push('goto: ' + String(error).slice(0, 160))
      }
      const info = await page.evaluate(() => {
        const h1 = document.querySelector('h1')
        return {
          h1: h1 ? h1.textContent.trim().slice(0, 40) : '',
          rootChildren: document.querySelector('#root')?.children.length ?? 0,
          textLen: document.body.innerText.replace(/\s+/g, ' ').trim().length,
          scrollWidth: document.documentElement.scrollWidth,
          innerWidth: window.innerWidth,
        }
      })
      results.push({
        viewport: width,
        path,
        status,
        ...info,
        overflow: info.scrollWidth > info.innerWidth + 1,
        errors: errors.slice(0, 4),
      })
      if (width === 390 && path === '/') {
        await page.screenshot({ path: 'e2e-local/prod-v13166-mobile.png' }).catch(() => {})
      }
      if (width === 1440 && path === '/') {
        await page.screenshot({ path: 'e2e-local/prod-v13166-desktop.png' }).catch(() => {})
      }
      await page.close()
    }
    await context.close()
  }
  await browser.close()
  console.log(JSON.stringify(results, null, 2))
  // 未登录时 /api/user/self 的 401 是预期噪声，不算失败
  const realErrors = (list) => list.filter((e) => !/status of 401/.test(e))
  const failed = results.filter(
    (r) => r.status !== 200 || !r.h1 || r.overflow || realErrors(r.errors).length > 0
  )
  if (failed.length) {
    console.error('FAIL:\n' + JSON.stringify(failed, null, 2))
    process.exit(1)
  }
  console.log('PASS: ' + results.length + ' 次导航全部 200、h1 有内容、无 console error、无横向溢出')
}

main().catch((error) => {
  console.error(error)
  process.exit(1)
})
