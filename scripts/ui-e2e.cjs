// 真实浏览器 UI E2E（本地实例，Playwright）。
//
// 覆盖本轮改动过的页面：训练数据管理页（Tabs + 分页布局）、Web 防护页（分页根因修复）、
// 运营面板、个人资料页（未配置云盘时**不出现**采集开关）。
//
// 用法：
//   1) 起本地实例（见 scripts/smoke-http.cjs 头部说明）
//   2) node scripts/ui-e2e.cjs http://127.0.0.1:18123
//
// 不联网、不调用付费 API、不写生产。
const { chromium } = require('../web/node_modules/playwright')

const BASE = process.argv[2] || 'http://127.0.0.1:18123'
const USER = 'smoke'
const PASS = 'SmokeTest123456'

let pass = 0
let fail = 0
const failures = []

function check(name, cond, detail) {
  if (cond) {
    pass++
    console.log(`  ✅ ${name}`)
  } else {
    fail++
    failures.push(name)
    console.log(`  ❌ ${name}${detail ? ' — ' + detail : ''}`)
  }
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE })
  // 强制英文：断言基于英文源串，而应用默认按浏览器语言（本机 zh）渲染。
  // 用 i18next 的 localStorage 键（src/i18n/config.ts 的 LANGUAGE_STORAGE_KEY）。
  await context.addInitScript(() => {
    try {
      localStorage.setItem(i18nextLng, en)
    } catch {
      /* 隐私模式下可能不可用，断言会退化为按中文匹配 */
    }
  })

  // 先走 API 登录：refresh cookie 会落进这个 context，
  // SPA 的 resolveAuthentication() 随后即可用。避免依赖登录页选择器。
  const setup = await context.request.post('/api/setup', {
    data: {
      username: USER,
      password: PASS,
      confirmPassword: PASS,
      SelfUseModeEnabled: false,
      DemoSiteEnabled: false,
    },
  })
  void setup

  const login = await context.request.post('/api/user/login', {
    data: { username: USER, password: PASS },
  })
  const loginJson = await login.json().catch(() => null)
  check('API 登录成功（为浏览器提供会话）', loginJson?.success === true, JSON.stringify(loginJson).slice(0, 160))

  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })
  page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message))

  // ---------- 1. 训练数据管理页 ----------
  console.log('\n[1] /training-data（管理页：SectionPageLayout + Tabs）')
  await page.goto('/training-data', { waitUntil: 'networkidle' })
  // 显式等待 tab 渲染：页面是懒加载 + i18n 异步初始化，
  // 固定 sleep 在冷启动时不够（实测 domcontentloaded+2.5s 仍有标签未渲染）。
  await page
    .getByRole('tab')
    .first()
    .waitFor({ timeout: 20000 })
    .catch(() => {})
  await page.waitForTimeout(800)

  const bodyText = (await page.locator('body').innerText().catch(() => '')) || ''
  check('页面不是空白（SPA 已渲染）', bodyText.length > 40, `len=${bodyText.length}`)
  // 语言无关断言：文案会随 locale 变化（zh/en），结构不会。
  // 早先按英文文案断言，在 zh 环境下全部误报失败。
  const tabCount = await page.getByRole('tab').count()
  check('管理页有 4 个 Tab（概览/云盘文件/导出/深度分析）', tabCount === 4, `tabCount=${tabCount}`)

  // 点击第一个 Tab 之后的切换，必须真的换内容（不是死按钮）
  const tabs = page.getByRole('tab')
  if (tabCount >= 2) {
    const before = bodyText
    await tabs.nth(1).click()
    await page.waitForTimeout(1500)
    const after = (await page.locator('body').innerText().catch(() => '')) || ''
    check('点击第 2 个 Tab 后内容变化（Tab 真实生效）', after !== before && after.length > 40)
  } else {
    check('点击第 2 个 Tab 后内容变化（Tab 真实生效）', false, '没有可点击的 Tab')
  }

  // ---------- 2. Web 防护页 ----------
  console.log('\n[2] /web-protection（分页根因修复：SectionPageLayout）')
  await page.goto('/web-protection', { waitUntil: 'networkidle' })
  await page
    .getByRole('tab')
    .first()
    .waitFor({ timeout: 20000 })
    .catch(() => {})
  await page.waitForTimeout(800)
  const wpText = (await page.locator('body').innerText().catch(() => '')) || ''
  check('Web 防护页渲染', wpText.length > 40, `len=${wpText.length}`)
  // 语言无关：三个 Tab 的结构存在即可（文案随 locale 变化）。
  const wpTabCount = await page.getByRole('tab').count()
  check('Web 防护页有 3 个 Tab（设置/请求日志/封禁 IP）', wpTabCount === 3, `tabCount=${wpTabCount}`)
  // 页面外壳必须带标题区（SectionPageLayout 的修复点：有了它才有页脚容器）
  check(
    '页面包含标题区（SectionPageLayout 生效）',
    /Web Protection|Web 防护/i.test(wpText)
  )

  // ---------- 3. 运营面板 ----------
  console.log('\n[3] /ops-panel')
  await page.goto('/ops-panel', { waitUntil: 'networkidle' })
  await page.waitForTimeout(2000)
  const opsText = (await page.locator('body').innerText().catch(() => '')) || ''
  check('运营面板渲染', opsText.length > 40, `len=${opsText.length}`)
  check('出现消耗/排行榜相关文案', /Consumption|Ranking|Quota|消耗|排行/i.test(opsText))

  // ---------- 4. 个人资料页：未配置云盘时不得出现采集开关 ----------
  console.log('\n[4] /profile（未配置云盘 → 不渲染采集开关）')
  await page.goto('/profile', { waitUntil: 'networkidle' })
  await page.waitForTimeout(2500)
  const profText = (await page.locator('body').innerText().catch(() => '')) || ''
  check('个人资料页渲染', profText.length > 40, `len=${profText.length}`)
  check(
    '未配置云盘时**不出现**采集开关卡片（中英文都不得出现）',
    !/Training Data Collection|训练数据采集/i.test(profText),
    '出现了本应隐藏的采集开关'
  )

  // ---------- 5. 移动端 320px 无横向溢出 ----------
  console.log('\n[5] 移动端 320px')
  const mobile = await browser.newContext({ baseURL: BASE, viewport: { width: 320, height: 720 } })
  await mobile.addInitScript(() => {
    try {
      localStorage.setItem(i18nextLng, en)
    } catch {
      /* ignore */
    }
  })
  const mPage = await mobile.newPage()
  // 复用已登录 storage/cookie：把 cookie 带过去
  const cookies = await context.cookies()
  await mobile.addCookies(cookies)
  for (const path of ['/training-data', '/web-protection']) {
    await mPage.goto(path, { waitUntil: 'networkidle' })
    await mPage.waitForTimeout(1500)
    const overflow = await mPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth
    )
    check(`${path} 在 320px 下无横向溢出`, overflow <= 1, `overflow=${overflow}px`)
  }
  await mobile.close()

  // ---------- 6. 控制台无致命错误 ----------
  console.log('\n[6] 控制台')
  const fatal = consoleErrors.filter((e) => !/favicon|404|Failed to fetch|NetworkError/i.test(e))
  check('无未预期的运行时错误', fatal.length === 0, fatal.slice(0, 3).join(' | '))

  await browser.close()

  console.log(`\n=== UI 结果：${pass} 通过 / ${fail} 失败 ===`)
  if (fail) {
    console.log('失败项：')
    failures.forEach((f) => console.log('  - ' + f))
    process.exit(1)
  }
  process.exit(0)
}

main().catch((e) => {
  console.error('脚本异常：', e)
  process.exit(1)
})
