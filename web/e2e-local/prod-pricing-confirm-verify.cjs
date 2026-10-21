/**
 * N3 验收：改「每单位额度 / 汇率」后保存会弹 L3 确认框，且必须逐字输入 QUOTA。
 * 只验证交互，**不真正提交**（点击取消）。
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER
const PASS = process.env.PROD_VERIFY_PASS || 'TempVerify!2026#Qz'

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()

  await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('input[name=username]', { timeout: 30000 })
  await page.fill('input[name=username]', USER)
  await page.fill('input[name=password]', PASS)
  await page.click('button[type=submit]')
  await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 })

  await page.goto(BASE + '/system-settings/billing/currency', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForTimeout(5000)

  // 找「每单位额度 / Quota Per Unit」的数字输入（按 label 关联）
  const inputs = page.locator('input')
  const count = await inputs.count()
  let target = null
  for (let i = 0; i < count; i += 1) {
    const el = inputs.nth(i)
    const id = await el.getAttribute('id')
    if (!id) continue
    const label = await page.locator(`label[for="${id}"]`).first().innerText().catch(() => '')
    if (/每单位额度|Quota Per Unit/i.test(label)) {
      target = el
      break
    }
  }

  const found = Boolean(target)
  let dialogText = ''
  let disabledBefore = null
  let enabledAfter = null
  if (found) {
    const current = await target.inputValue()
    await target.fill(String(Number(current || '500000') + 1))
    await page.waitForTimeout(300)
    await page.getByRole('button', { name: /保存更改|Save Changes|保存/ }).first().click()
    await page.waitForTimeout(1200)
    const dialog = page.locator('[role=alertdialog]')
    if ((await dialog.count()) > 0) {
      dialogText = (await dialog.first().innerText()).replace(/\s+/g, ' ').slice(0, 120)
      const confirmBtn = dialog.first().getByRole('button', { name: /保存更改|Save Changes/i }).first()
      disabledBefore = await confirmBtn.isDisabled()
      const typedInput = dialog.first().locator('input')
      await typedInput.fill('QUOTA')
      await page.waitForTimeout(300)
      enabledAfter = !(await confirmBtn.isDisabled())
      await page.screenshot({ path: 'e2e-local/nielsen-pricing-confirm.png' }).catch(() => {})
      await page.keyboard.press('Escape')
    }
  }

  await browser.close()
  console.log(JSON.stringify({ foundQuotaInput: found, dialogText, disabledBeforeTyping: disabledBefore, enabledAfterTyping: enabledAfter }, null, 1))
  if (!found || !/QUOTA|确认/i.test(dialogText) || disabledBefore !== true || enabledAfter !== true) {
    console.error('FAIL: L3 输入确认不生效')
    process.exit(1)
  }
  console.log('PASS: 改额度单位保存会弹 L3 确认，未输入 QUOTA 时按钮禁用、输入后启用')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
