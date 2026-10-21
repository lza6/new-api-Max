/**
 * 生产管理员登录态验收（临时账号，跑完即删）。
 *
 * 手法见 memory `new-api-prod-ui-e2e-tunnel`：
 *  - 隧道指 443（Secure cookie 才会下发），Chromium 用 host-resolver-rules 映射域名
 *  - 必须走真实 /sign-in 表单；`context.request.post` 不走 host-resolver-rules
 *  - 登录后复用同一个标签页（新标签页有鉴权首帧竞态）
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER || 'zz-verify-temp'
const PASS = 'TempVerify!2026#Qz'
const PATHS = [
  '/dashboard/models',
  '/system-settings/site/system-info',
  '/system-settings/operations/behavior',
  '/system-settings/security/rate-limit',
  '/wallet',
  '/keys',
]

;(async () => {
  const browser = await chromium.launch({
    args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'],
  })
  const context = await browser.newContext({
    baseURL: BASE,
    ignoreHTTPSErrors: true,
    viewport: { width: 1440, height: 900 },
  })
  const page = await context.newPage()
  const results = []
  const errors = []
  page.on('pageerror', (e) => errors.push('pageerror: ' + String(e).slice(0, 180)))
  page.on('console', (m) => {
    if (m.type() === 'error' && !/status of 40[13]/.test(m.text())) {
      errors.push('console: ' + m.text().slice(0, 180))
    }
  })

  await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('input[name=username]', { timeout: 30000 })
  await page.fill('input[name=username]', USER)
  await page.fill('input[name=password]', PASS)
  await page.click('button[type=submit]')
  await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 }).catch(() => {})
  const loginUrl = page.url()
  const loggedIn = !loginUrl.includes('/sign-in')
  results.push({ step: 'login', url: loginUrl, loggedIn, errors: [...errors] })

  if (!loggedIn) {
    console.log(JSON.stringify(results, null, 2))
    await browser.close()
    process.exit(1)
  }

  for (const path of PATHS) {
    const before = errors.length
    await page.goto(BASE + path, { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 30000 }).catch(() => {})
    await page.waitForTimeout(2500)
    const info = await page.evaluate(() => {
      const text = document.body.innerText.replace(/\s+/g, ' ').trim()
      return {
        h1: document.querySelector('h1')?.textContent?.trim().slice(0, 40) || '',
        textLen: text.length,
        isErrorPage: text.includes('糟糕') || /(^|\s)500(\s|$)/.test(text),
        preview: text.slice(0, 90),
      }
    })
    results.push({ step: path, url: page.url(), ...info, errors: errors.slice(before) })
    await page.screenshot({ path: `e2e-local/prod-admin-${path.replaceAll('/', '_')}.png` }).catch(() => {})
  }

  await browser.close()
  console.log(JSON.stringify(results, null, 2))
  const bad = results.filter((r) => r.isErrorPage || r.textLen < 120 || (r.errors && r.errors.length > 0) || !r.url.includes(r.step ?? ''))
  const failed = results.filter((r) => r.step !== 'login' && (r.isErrorPage || r.textLen < 120 || (r.errors || []).length > 0))
  if (!loggedIn || failed.length) {
    console.error('FAIL:\n' + JSON.stringify(failed.length ? failed : bad, null, 2))
    process.exit(1)
  }
  console.log('PASS: 管理员登录 + ' + PATHS.length + ' 个后台页面均正常渲染，无 error boundary、无 console error')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
