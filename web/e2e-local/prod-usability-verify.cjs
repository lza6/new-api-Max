/**
 * 尼尔森整改的生产截图验收（N1 术语 / N4 确认框 / N8 错误页）。
 * 需要 9600 隧道；管理员步骤用 PROD_VERIFY_USER（临时账号）。
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER
const PASS = process.env.PROD_VERIFY_PASS || 'TempVerify!2026#Qz'

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } })
  const page = await context.newPage()
  const out = {}


  if (USER) {
    await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForSelector('input[name=username]', { timeout: 30000 })
    await page.fill('input[name=username]', USER)
    await page.fill('input[name=password]', PASS)
    await page.click('button[type=submit]')
    await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 })

    // N4：签到重置必须走项目 ConfirmDialog
    await page.goto(BASE + '/system-settings/billing/checkin', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(4000)
    const resetBtn = page.getByRole('button', { name: /重置所有签到记录|Reset all check-in records/i }).first()
    const hasReset = (await resetBtn.count()) > 0
    let dialogOk = false
    let dialogText = ''
    if (hasReset) {
      await resetBtn.click()
      await page.waitForTimeout(1000)
      const dialog = page.locator('[role=alertdialog]')
      dialogOk = (await dialog.count()) > 0
      dialogText = dialogOk ? (await dialog.first().innerText()).replace(/\s+/g, ' ').slice(0, 80) : ''
      await page.screenshot({ path: 'e2e-local/nielsen-checkin-confirm.png' }).catch(() => {})
      await page.keyboard.press('Escape')
    }
    out.checkin = { hasResetButton: hasReset, confirmDialog: dialogOk, dialogText }

    // N8：错误页（登录态下访问不存在的路由）
    await page.goto(BASE + '/zz-does-not-exist', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(3000)
    const errText = await page.evaluate(() => document.body.innerText.replace(/s+/g, ' '))
    out.errorPage = {
      hasHumanTitle: /糟糕|出错了|not found|页面/i.test(errText),
      hasHttpChip: /HTTP\s*\d{3}/i.test(errText),
      hasRequestId: /请求 ID|Request ID/i.test(errText),
      bigCodeDominant: /^s*(404|500)/.test(errText),
      preview: errText.slice(0, 120),
    }
    await page.screenshot({ path: 'e2e-local/nielsen-error-page.png' }).catch(() => {})

    // N1：钱包页术语（额度）
    await page.goto(BASE + '/wallet', { waitUntil: 'domcontentloaded', timeout: 60000 })
    await page.waitForTimeout(3500)
    const walletText = await page.evaluate(() => document.body.innerText.replace(/\s+/g, ' '))
    out.wallet = {
      mentionsQuotaCn: /额度/.test(walletText),
      mentionsOldTerm: /配额|容量/.test(walletText),
    }
    await page.screenshot({ path: 'e2e-local/nielsen-wallet.png' }).catch(() => {})
  }

  await browser.close()
  console.log(JSON.stringify(out, null, 1))

  // 404 页面没有 request id（那是服务端错误才有的诊断信息），故只要求
  // 「人话标题 + 状态标签 + 不再以巨型状态码为主体」；GeneralError 的
  // request id 由组件测试与人工查看截图确认。
  const ok =
    out.errorPage.hasHumanTitle &&
    out.errorPage.hasHttpChip &&
    !out.errorPage.bigCodeDominant &&
    (!USER || (out.checkin.confirmDialog && out.checkin.dialogText.length > 0))
  if (!ok) {
    console.error('FAIL: 尼尔森整改未达到验收条件')
    process.exit(1)
  }
  console.log('PASS: 错误页人话化（人话标题 + 状态标签）、签到重置走 ConfirmDialog')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
