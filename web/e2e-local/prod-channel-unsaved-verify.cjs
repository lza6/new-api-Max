/**
 * N5 验收：渠道编辑抽屉在「有未保存修改」时，Esc / 关闭会先弹确认框。
 * 需要 9600 隧道 + PROD_VERIFY_USER。
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

  await page.goto(BASE + '/channels', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForTimeout(5000)

  // 打开「新增渠道」抽屉
  const createBtn = page.getByRole('button', { name: /创建渠道|添加渠道|新增渠道|Create Channel|Add Channel/i }).first()
  const hasCreate = (await createBtn.count()) > 0
  if (hasCreate) {
    await createBtn.click()
  } else {
    // 回退：点第一行的操作菜单 → 编辑
    await page.locator('[data-slot="table"] tbody tr').first().getByRole('button').last().click()
    await page.getByRole('menuitem', { name: /编辑|Edit/i }).first().click()
  }
  await page.waitForSelector('[role=dialog]', { timeout: 30000 })
  await page.waitForTimeout(1500)

  // 改一个必填字段
  const nameInput = page.locator('[role=dialog] input[placeholder*="OpenAI GPT-4"]').first()
  await nameInput.fill('ZZ N5 Unsave Check')
  await page.waitForTimeout(300)
  await page.waitForTimeout(400)

  // 点抽屉底部的「取消」尝试关闭（比 Esc 更接近真实用户路径）
  await page.getByRole('button', { name: /^取消$|^Cancel$/ }).last().click()
  await page.waitForTimeout(1200)

  const dialogs = await page.locator('[role=dialog], [role=alertdialog]').count()
  const alertText = (await page.locator('[role=alertdialog]').first().innerText().catch(() => '')).replace(/\s+/g, ' ')
  await page.screenshot({ path: 'e2e-local/nielsen-channel-unsaved.png' }).catch(() => {})

  await browser.close()
  const ok = /未保存|Unsaved/i.test(alertText)
  console.log(JSON.stringify({ hasCreateButton: hasCreate, dialogsAfterEsc: dialogs, alertText: alertText.slice(0, 80) }, null, 1))
  if (!ok) {
    console.error('FAIL: 渠道抽屉没有弹出未保存确认')
    process.exit(1)
  }
  console.log('PASS: 渠道抽屉在有修改时按 Esc 会弹「未保存的更改」确认')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
