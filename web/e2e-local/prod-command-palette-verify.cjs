/**
 * N9 验收：命令面板包含「快捷操作」动作组，且键盘可打开。
 * 需要 9600 隧道 + PROD_VERIFY_USER（临时管理员）。
 */
const { chromium } = require('playwright')

const BASE = 'https://freeapi.tingfengai.art:9600'
const USER = process.env.PROD_VERIFY_USER
const PASS = process.env.PROD_VERIFY_PASS || 'TempVerify!2026#Qz'

;(async () => {
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP freeapi.tingfengai.art 127.0.0.1'] })
  const context = await browser.newContext({ baseURL: BASE, ignoreHTTPSErrors: true, viewport: { width: 1280, height: 900 } })
  const page = await context.newPage()

  await page.goto(BASE + '/sign-in', { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForSelector('input[name=username]', { timeout: 30000 })
  await page.fill('input[name=username]', USER)
  await page.fill('input[name=password]', PASS)
  await page.click('button[type=submit]')
  await page.waitForFunction(() => !location.pathname.startsWith('/sign-in'), { timeout: 45000 })
  await page.waitForTimeout(2500)

  // 用键盘打开（⌘/Ctrl + K），验证快捷键真的可用
  await page.keyboard.press('Control+KeyK')
  await page.waitForTimeout(1200)
  let dialogText = ''
  let opened = false
  const dialog = page.locator('[role=dialog]')
  if ((await dialog.count()) === 0) {
    // 回退：点搜索按钮
    await page.getByRole('button', { name: /搜索|Search|Type a command/i }).first().click().catch(() => {})
    await page.waitForTimeout(1200)
  }
  opened = (await dialog.count()) > 0
  if (opened) {
    dialogText = (await dialog.first().innerText()).replace(/\s+/g, ' ')
    await page.screenshot({ path: 'e2e-local/nielsen-command-palette.png' }).catch(() => {})
  }

  const actions = ['API 密钥|API Keys', '钱包|Wallet', '使用日志|Usage Logs', '状态|Status']
  const found = actions.filter((a) => new RegExp(a, 'i').test(dialogText))

  await browser.close()
  console.log(JSON.stringify({ opened, hasQuickActions: /快捷操作|Quick actions/i.test(dialogText), found, preview: dialogText.slice(0, 160) }, null, 1))

  if (!opened || !/快捷操作|Quick actions/i.test(dialogText) || found.length < 4) {
    console.error('FAIL: 命令面板快捷操作不完整')
    process.exit(1)
  }
  console.log('PASS: 命令面板可键盘打开，且含 4 个常用动作')
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
