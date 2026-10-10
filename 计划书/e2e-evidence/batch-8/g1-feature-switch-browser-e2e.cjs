/*
 * Batch-8 / G1「实验功能（Feature Switches）」管理页 —— 真实浏览器 E2E。
 *
 * 覆盖：注册首个用户（自动成为 root）→ 登录 → 进入新页面 → 断言 13 个开关渲染
 *      → 切换一个低风险开关 → 断言即时生效 → 刷新断言持久化 → 复位 → 截图。
 *
 * 运行：node 计划书/e2e-evidence/batch-8/g1-feature-switch-browser-e2e.cjs [baseUrl]
 * 依赖：web/node_modules/playwright
 */
const path = require('node:path')
const fs = require('node:fs')

const WEB = path.resolve(__dirname, '../../../web')
const { chromium } = require(path.join(WEB, 'node_modules', 'playwright'))

const BASE = process.argv[2] || 'http://127.0.0.1:3099'
const OUT = __dirname
const USER = 'e2eadmin' // ≤12 字符：/api/setup 的用户名校验上限
const PASS = 'B8e2e!Passw0rd'

const results = []
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail: detail === undefined ? '' : String(detail) })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  :: ' + detail : ''}`)
}

async function apiJson(url, options) {
  const res = await fetch(url, options)
  const text = await res.text()
  let json = null
  try {
    json = JSON.parse(text)
  } catch {
    json = null
  }
  return { status: res.status, json, text }
}

;(async () => {
  // 1) 初始化实例：POST /api/setup 建 root 账号并把 constant.Setup 置真。
  //    若实例已初始化（返回 false + "系统已经初始化完成"），视为通过 —— 重复跑用例时不阻塞。
  const setup = await apiJson(`${BASE}/api/setup`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username: USER,
      password: PASS,
      confirmPassword: PASS,
    }),
  })
  const setupOk =
    setup.status === 200 &&
    (setup.json?.success === true ||
      (setup.json?.message || '').includes('已经初始化完成'))
  check('instance initialized (POST /api/setup)', setupOk,
    `status=${setup.status} body=${setup.text.slice(0, 140)}`)

  const browser = await chromium.launch({
    args: ['--no-proxy-server', '--proxy-bypass-list=*'],
    proxy: { server: 'direct://' },
  })
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1100 } })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e).slice(0, 200)))

  // 2) 在**浏览器上下文内**登录（这样会话 Cookie 才落在浏览器里）。
  await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  const login = await page.evaluate(
    async (creds) => {
      const r = await fetch('/api/user/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(creds),
      })
      const text = await r.text()
      let json = null
      try {
        json = JSON.parse(text)
      } catch {
        json = null
      }
      return { status: r.status, json, text }
    },
    { username: USER, password: PASS }
  )
  check('login', login.status === 200 && login.json && login.json.success === true,
    `status=${login.status} msg=${login.json && login.json.message}`)

  // 3) 直接进新管理页
  await page.goto(`${BASE}/system-settings/feature-switches/switches`, {
    waitUntil: 'domcontentloaded',
    timeout: 60000,
  })
  await page.waitForTimeout(6000)

  const body = await page.evaluate(() => document.body.innerText)
  check('page not redirected to login', !page.url().includes('/login'), page.url())
  check('page shows section title', body.includes('实验功能') || body.includes('Feature Switches'))

  // 4) 13 个开关 + 关键 UI 元素
  const KEYS = [
    'COMPLEXITY_ROUTING', 'TOOL_DRAWER_ENABLED', 'RELAY_AUDIT_ENABLED',
    'POLICY_ENGINE_MODE', 'CHANNEL_HEALTH_WEIGHTED_LB', 'DOMAIN_ROUTE_ENABLED',
    'MEMORY_INJECTION_ENABLED', 'CHANNEL_CIRCUIT_BREAKER', 'CHANNEL_KEY_ENCRYPTION',
    'PASSWORD_LOGIN_ENCRYPTION_ENABLED', 'CATALOG_SYNC_TASK_ENABLED',
    'ERROR_LOG_ENABLED', 'GET_MEDIA_TOKEN_NOT_STREAM',
  ]
  const missingKeys = KEYS.filter((k) => !body.includes(k))
  check('all 13 switches rendered', missingKeys.length === 0, `missing=${JSON.stringify(missingKeys)}`)
  check('effect metrics block present', body.includes('效果度量') || body.includes('Effect metrics'))
  check('risk badges present', body.includes('高风险') || body.includes('High risk'))
  check('rollback hint present', body.includes('回滚') || body.includes('Rollback'))
  check('chinese i18n applied', body.includes('实验功能'))

  await page.screenshot({ path: `${OUT}/01-feature-switches-page.png`, fullPage: true })

  // 5) 切换一个**低风险**开关（中继一致性自检），断言即时生效。
  //    Base UI 的 Switch 用 data-checked / data-unchecked（不是 data-state）。
  const cardOf = (key) =>
    page
      .locator('[data-slot="card"]')
      .filter({ hasText: key })
      .filter({ has: page.locator('[data-slot="switch"]') })
      .first()
  const isOn = (loc) => loc.evaluate((el) => el.hasAttribute('data-checked'))

  const card = cardOf('RELAY_AUDIT_ENABLED')
  const sw = card.locator('[data-slot="switch"]').first()
  const before = await isOn(sw)
  check('relay-audit switch is off initially', before === false, `on=${before}`)

  const cardTextBefore = await card.innerText()

  // 归一化初始状态：上一轮用例可能留下 configured=true（点关开关只写值、不清配置），
  // 先把残留配置通过 UI 的「重置为默认」清掉，让本用例可重复运行。
  const preReset = cardOf('RELAY_AUDIT_ENABLED').getByRole('button', { name: /重置为默认/ })
  if ((await preReset.count()) > 0) {
    await preReset.click()
    await page.waitForTimeout(2500)
  }

  // 「重置为默认」按钮只在服务端返回 configured=true 时渲染 —— 用它当作
  // 「服务端确实记下了管理员配置」的 UI 侧证据（接口侧真值由姊妹脚本验证）。
  const resetBtnBefore = await cardOf('RELAY_AUDIT_ENABLED')
    .getByRole('button', { name: /重置为默认/ })
    .count()
  check('reset button hidden while unconfigured', resetBtnBefore === 0, `count=${resetBtnBefore}`)

  await sw.click()
  await page.waitForTimeout(3500)
  const afterState = await isOn(sw)
  check('relay-audit switch turned on', afterState === true, `on=${afterState}`)

  // 「打开后看得见效果」——这是本页存在的主要理由，必须验到数字真的出现。
  const cardTextAfter = await card.innerText()
  const metricShown = /relay_audit_findings_total\s*=\s*\d+/.test(cardTextAfter)
  check('effect metric shows a number after enabling', metricShown,
    `after="${(cardTextAfter.match(/relay_audit_findings_total[^\n]*/) || [''])[0]}"`)
  const resetBtnAfter = await card.getByRole('button', { name: /重置为默认/ }).count()
  check('server reports the switch as configured (reset button appeared)',
    resetBtnAfter === 1, `count=${resetBtnAfter}`)
  await page.screenshot({ path: `${OUT}/02-relay-audit-enabled.png`, fullPage: false })

  // 6) 刷新 → 持久化（看「已在管理端配置」标记，而非仅看开关视觉）
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(6000)
  const cardTextReloaded = await cardOf('RELAY_AUDIT_ENABLED').innerText()
  check(
    'switch persisted after reload (configured in admin console)',
    cardTextReloaded.includes('已在管理端配置') ||
      cardTextReloaded.includes('Configured in the admin console'),
    cardTextReloaded.split('\n').slice(0, 12).join(' | ').slice(0, 200)
  )
  check('switch still on after reload',
    (await isOn(cardOf('RELAY_AUDIT_ENABLED').locator('[data-slot="switch"]').first())) === true, '')

  // 7) 用「重置为默认」按钮走一遍复位路径（同时覆盖 UI 的 reset 分支）。
  //    接口层真值（value/configured/effective 三个字段）由
  //    persistence-restart-e2e.cjs 以 Bearer 鉴权单独验证，并含跨进程重启。
  const resetBtn = cardOf('RELAY_AUDIT_ENABLED').getByRole('button', { name: /重置为默认/ })
  check('reset button present before reset', (await resetBtn.count()) === 1, '')
  await resetBtn.click()
  await page.waitForTimeout(3000)
  const cardTextReset = await cardOf('RELAY_AUDIT_ENABLED').innerText()
  check('reset returns the switch to the environment default',
    cardTextReset.includes('使用环境默认值') || cardTextReset.includes('Using the environment default'),
    cardTextReset.split('\n').slice(0, 10).join(' | ').slice(0, 180))

  // 8) 复位后开关应回到关闭（env 默认 false）—— 只断言，不再点击（避免把用例自身污染）
  const sw2 = cardOf('RELAY_AUDIT_ENABLED').locator('[data-slot="switch"]').first()
  const resetState = await isOn(sw2)
  check('switch is off again after reset', resetState === false, `on=${resetState}`)

  check('no uncaught page errors', errors.length === 0, JSON.stringify(errors.slice(0, 3)))

  await browser.close()

  const passed = results.filter((r) => r.ok).length
  const failed = results.length - passed
  const md = [
    `# Batch-8 / G1 浏览器 E2E 结果`,
    ``,
    `- 时间：${new Date().toISOString()}`,
    `- 目标：${BASE}`,
    `- 结果：**${passed}/${results.length} PASS**，失败 ${failed}`,
    ``,
    `| # | 断言 | 结果 | 详情 |`,
    `|---|---|---|---|`,
    ...results.map((r, i) => `| ${i + 1} | ${r.name} | ${r.ok ? '✅' : '❌'} | ${r.detail.replace(/\|/g, '\\|').slice(0, 160)} |`),
    ``,
    `截图：\`01-feature-switches-page.png\`（全页）、\`02-relay-audit-enabled.png\`（启用后）`,
    ``,
  ].join('\n')
  fs.writeFileSync(`${OUT}/RESULTS.md`, md)
  console.log(`\n=== ${passed}/${results.length} PASS ===`)
  process.exit(failed === 0 ? 0 : 1)
})().catch((e) => {
  console.error('E2E ERROR: ' + e.message)
  fs.writeFileSync(
    `${OUT}/RESULTS.md`,
    `# Batch-8 / G1 浏览器 E2E 结果\n\n**执行中断**：${e.message}\n\n已完成断言：\n` +
      results.map((r, i) => `${i + 1}. ${r.ok ? 'PASS' : 'FAIL'} ${r.name} ${r.detail}`).join('\n')
  )
  process.exit(1)
})
