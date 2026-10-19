// 导出列表分页 + 下载记录上限提示的真实浏览器验证（本地实例）。
// 前置：本地实例已启动，且已 seed 21 条导出 + 第一条导出的 100 条下载记录。
// 用法：node scripts/ui-e2e-export.cjs http://127.0.0.1:18123
const { chromium } = require('../web/node_modules/playwright')

const BASE = process.argv[2] || 'http://127.0.0.1:18123'
const USER = 'smoke'
const PASS = 'SmokeTest123456'

let pass = 0
let fail = 0
function check(name, cond, detail) {
  if (cond) {
    pass++
    console.log(`  ✅ ${name}`)
  } else {
    fail++
    console.log(`  ❌ ${name}${detail ? ' — ' + detail : ''}`)
  }
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE })
  const login = await context.request.post('/api/user/login', {
    data: { username: USER, password: PASS },
  })
  const loginJson = await login.json().catch(() => null)
  check('API 登录成功', loginJson?.success === true)

  const page = await context.newPage()
  await page.goto('/training-data', { waitUntil: 'networkidle' })
  await page.getByRole('tab').first().waitFor({ timeout: 20000 }).catch(() => {})
  await page.waitForTimeout(800)

  const tabs = page.getByRole('tab')
  const tabCount = await tabs.count()
  check('管理页有 4 个 Tab', tabCount === 4, `tabCount=${tabCount}`)

  // 第 3 个 Tab = 导出
  await tabs.nth(2).click()
  await page.waitForTimeout(1500)

  const body = (await page.locator('body').innerText().catch(() => '')) || ''
  check('导出列表加载（出现分页页码 1 / 2）', /\b1\s*\/\s*2\b/.test(body), body.slice(0, 300))
  check('显示总数 21', /21/.test(body))

  const next = page.getByRole('button', { name: /Next|下一页/ }).first()
  const nextCount = await next.count()
  check('存在“下一页”按钮', nextCount > 0)
  if (nextCount > 0) {
    await next.click()
    await page.waitForTimeout(1200)
    const body2 = (await page.locator('body').innerText().catch(() => '')) || ''
    check('翻页后显示 2 / 2', /\b2\s*\/\s*2\b/.test(body2), body2.slice(0, 300))
  }

  // 回第一页，打开第一条导出的下载记录
  const prev = page.getByRole('button', { name: /Previous|上一页/ }).first()
  if (await prev.count()) {
    await prev.click()
    await page.waitForTimeout(1000)
  }
  const history = page.getByRole('button', { name: /Download history|下载记录/ }).first()
  check('存在“下载记录”按钮', (await history.count()) > 0)
  if (await history.count()) {
    await history.click()
    await page.waitForTimeout(1500)
    const dialog = (await page.locator('body').innerText().catch(() => '')) || ''
    check('下载记录弹窗显示 100 条记录', /#\d+/.test(dialog) && /100/.test(dialog))
    check(
      '显示“仅最近 100 条”提示',
      /latest 100|最近 100|仅显示最近/.test(dialog),
      dialog.slice(0, 400)
    )
  }

  await page.screenshot({ path: '计划书/e2e-evidence/v13157-export-ui.png', fullPage: true })
  await browser.close()
  console.log(`\n=== 结果：${pass} 通过 / ${fail} 失败 ===`)
  process.exit(fail === 0 ? 0 : 1)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
