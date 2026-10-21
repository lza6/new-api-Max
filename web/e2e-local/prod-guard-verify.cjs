/**
 * 生产验收：设置页「未保存离开」保护（v1.3.170 共享层改造）。
 *
 * 场景：
 *   1) 打开站点信息，**不改任何东西** → 点侧栏另一个分区，应当直接跳转（不弹窗）
 *   2) 改一个字段 → 点侧栏另一个分区，应当弹出确认框；点「取消」留在原页
 *   3) 点「离开」才真的跳转（可选断言）
 *
 * 依赖：9600 隧道；PROD_VERIFY_USER / PROD_VERIFY_PASS 环境变量（临时管理员）。
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER
const PASS = process.env.PROD_VERIFY_PASS || 'TempVerify!2026#Qz'

if (!USER) {
  console.error('PROD_VERIFY_USER is required')
  process.exit(2)
}

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  const results = []

  await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('input[name=username]', { timeout: 30000 })
  await page.fill('input[name=username]', USER)
  await page.fill('input[name=password]', PASS)
  await page.click('button[type=submit]')
  await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 })

  const openSiteInfo = async () => {
    await page.goto(BASE + '/system-settings/site/system-info', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForFunction(() => (document.querySelector('#root')?.children.length ?? 0) > 0, { timeout: 30000 })
    // 设置页表单是懒加载的，等真实输入框出现再操作（2s 不够）
    await page.waitForSelector('input:not([type=hidden]):not([type=checkbox]):not([type=radio])', { timeout: 30000 })
  }

  // 1) 未修改 → 直接跳转
  await openSiteInfo()
  const noticeLink = page.locator('a[href$="/system-settings/site/notice"]').first()
  await noticeLink.click()
  await page.waitForTimeout(1500)
  results.push({
    step: 'clean-navigation',
    url: page.url(),
    navigated: page.url().includes('/system-settings/site/notice'),
    dialog: await page.locator('[role=dialog], [role=alertdialog]').count(),
  })

  // 2) 修改后 → 应弹确认框
  await openSiteInfo()
  const input = page.locator('input:not([type=hidden]):not([type=checkbox]):not([type=radio])').first()
  await input.fill('ZZ Guard Check')
  await page.waitForTimeout(300)
  await page.locator('a[href$="/system-settings/site/notice"]').first().click()
  await page.waitForTimeout(1500)
  const dialog = page.locator('[role=dialog], [role=alertdialog]')
  const dialogCount = await dialog.count()
  const dialogText = dialogCount ? (await dialog.first().innerText()).slice(0, 60) : ''
  await page.screenshot({ path: 'e2e-local/prod-guard-dialog.png' }).catch(() => {})
  results.push({
    step: 'dirty-navigation',
    url: page.url(),
    stayed: page.url().includes('/system-settings/site/system-info'),
    dialog: dialogCount,
    dialogText,
  })

  await browser.close()
  console.log(JSON.stringify(results, null, 1))

  const clean = results[0]
  const dirty = results[1]
  const ok = clean.navigated && clean.dialog === 0 && dirty.dialog > 0 && dirty.stayed
  if (!ok) {
    console.error('FAIL: 未保存保护行为不符合预期')
    process.exit(1)
  }
  console.log('PASS: 无改动可直接跳转；有改动会弹确认框并留在原页')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
