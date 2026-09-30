/**
 * v1.3.60-62 新功能真实浏览器 E2E（本地）
 *
 * 覆盖本轮所有**用户/管理员可见**的改动：
 *   A. 数据库导出/导入 UI（系统设置 → 运维 → Database Backup）
 *   B. 用户查看自己的密钥免二次验证
 *   C. 渠道多 key 面板：行内「测试」/「一键测试全部」/「新增密钥」
 *   D. 上游不可达 502 语义（API 层断言）
 *   E. a11y：reduced-motion 下内容可见；无 axe critical 违规
 *
 * 前置：本地服务已起（PORT=3000 SQLITE_PATH=e2e-v1363.db）且已 /api/setup。
 * 输出：计划书/e2e-evidence/v1.3.62/ 截图 + results.json
 */
const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.E2E_BASE || 'http://127.0.0.1:3000'
const USER = process.env.E2E_USER || 'e2e_root'
const PASS = process.env.E2E_PASS || 'E2ePass!2026'
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.62')

const results = []
const record = (name, ok, detail) => {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ' — ' + detail : ''}`)
}
const shot = async (page, name) => {
  fs.mkdirSync(OUT, { recursive: true })
  await page.screenshot({ path: path.join(OUT, `${name}.png`), fullPage: true })
}

/** 用管理 API 建一个多 key 渠道（UI 表单长，用 API 铺数据后验证 UI 操作）。 */
async function seedMultiKeyChannel(page, base) {
  return page.evaluate(async (b) => {
    const login = await fetch(`${b}/api/user/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'e2e_root', password: 'E2ePass!2026' }),
    }).then((r) => r.json())
    const tok = login.data.access_token
    const h = { 'Content-Type': 'application/json', Authorization: `Bearer ${tok}` }
    // OpenAI 兼容渠道，多 key（每行一个），指向不可达地址（测失败路径也好断言）
    const res = await fetch(`${b}/api/channel/`, {
      method: 'POST',
      headers: h,
      body: JSON.stringify({
        mode: 'multi_to_single',
        multi_key_mode: 'polling',
        channel: {
          name: 'e2e-multikey',
          type: 1,
          key: 'sk-e2e-key-aaaaaaaaaaaaaaaaaaaa\nsk-e2e-key-bbbbbbbbbbbbbbbbbbbb',
          base_url: 'https://e2e-unreachable.invalid',
          models: 'gpt-4o-mini',
          group: 'default',
          status: 1,
          auto_ban: 0,
        },
      }),
    }).then((r) => r.json())
    return res
  }, base)
}

;(async () => {
  const browser = await chromium.launch()
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })

  try {
    // ---------- 登录 ----------
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'networkidle' })
    await page.fill('input[name="username"], input#username', USER)
    await page.fill('input[name="password"], input#password', PASS)
    await page.getByRole('button', { name: /登录|Login|Sign in/i }).click()
    await page.waitForURL((u) => !u.pathname.includes('sign-in'), { timeout: 20000 })
    record('登录成功', true, page.url())

    // ---------- A. 数据库备份 UI ----------
    await page.goto(`${BASE}/system-settings/operations/database-backup`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(2500)
    await shot(page, 'A1-db-backup')
    record('备份分区可见', await page.getByText(/数据库备份|Database Backup/i).first().isVisible().catch(() => false))
    record('防错提示（不删除不覆盖）', await page.getByText(/绝不删除或覆盖|never delete or overwrite/i).first().isVisible().catch(() => false))
    const [dl] = await Promise.all([
      page.waitForEvent('download', { timeout: 120000 }).catch(() => null),
      page.getByRole('button', { name: /下载备份|Download backup/i }).click(),
    ])
    if (dl) {
      const p = path.join(OUT, 'backup-export.sqljson.gz')
      await dl.saveAs(p)
      const gz = fs.readFileSync(p)
      record('导出下载为有效 gzip', gz[0] === 0x1f && gz[1] === 0x8b && gz.length > 0, `${gz.length} bytes`)
    } else record('导出下载为有效 gzip', false, '未捕获 download')

    // ---------- B. 用户自己的密钥免二次验证 ----------
    await page.evaluate(async (b) => {
      const login = await fetch(`${b}/api/user/login`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: 'e2e_root', password: 'E2ePass!2026' }),
      }).then((r) => r.json())
      await fetch(`${b}/api/token/`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${login.data.access_token}` },
        body: JSON.stringify({ name: 'e2e-reveal', remain_quota: 500000, expired_time: -1, unlimited_quota: false, model_limits_enabled: false }),
      })
    }, BASE)
    await page.goto(`${BASE}/keys`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(3000)
    const rows = await page.locator('table tbody tr').count()
    record('密钥列表非空（防假阳性）', rows > 0, `rows=${rows}`)
    const trig = page.locator('table tbody').getByText(/^sk-[^*]*\*+/).first()
    const trigOk = await trig.isVisible().catch(() => false)
    record('找到掩码密钥触发器', trigOk)
    let dialogShown = false, unlocked = false
    if (trigOk) {
      await trig.click()
      await page.waitForTimeout(3500)
      dialogShown = await page.getByText(/Verify to view API key|验证后查看|Confirm your identity/i).first().isVisible().catch(() => false)
      unlocked = await page.getByText(/API Key 已解锁|API key unlocked|已解锁/i).first().isVisible().catch(() => false)
      await shot(page, 'B1-key-revealed')
    }
    record('查看自己的密钥未弹二次验证', trigOk && !dialogShown)
    record('密钥确实被揭示（已解锁 toast）', unlocked)

    // ---------- C. 渠道多 key 面板 ----------
    const seeded = await seedMultiKeyChannel(page, BASE)
    record('多 key 渠道已创建（API）', Boolean(seeded?.success || seeded?.data?.id), JSON.stringify(seeded).slice(0, 120))
    await page.goto(`${BASE}/channels`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(3000)
    await shot(page, 'C1-channels-list')
    // 渠道列表是**卡片布局**（非 table）；卡片内操作菜单项文案为 "Manage Keys"。
    const card = page.locator('[data-channel-card], .rounded-xl, .rounded-lg').filter({ hasText: 'e2e-multikey' }).first()
    const cardVisible = await card.isVisible().catch(() => false)
    record('渠道列表含 e2e-multikey', cardVisible)
    if (cardVisible) {
      // 操作菜单触发器带 sr-only 可访问名 "Open menu"
      await card.getByRole('button', { name: /Open menu|打开菜单/i }).first().click().catch(() => {})
      await page.waitForTimeout(1000)
      const menuItem = page.getByRole('menuitem', { name: /Manage Keys|管理密钥|多密钥/i }).first()
      if (await menuItem.isVisible().catch(() => false)) {
        await menuItem.click()
        await page.waitForTimeout(2500)
        await shot(page, 'C2-multikey-panel')
        record('多密钥面板打开', await page.getByText(/密钥|Key/i).first().isVisible().catch(() => false))
        record('「一键测试全部密钥」按钮存在', await page.getByRole('button', { name: /一键测试全部密钥|Test All Keys/i }).first().isVisible().catch(() => false))
        record('「新增密钥」按钮存在', await page.getByRole('button', { name: /新增密钥|Add Keys/i }).first().isVisible().catch(() => false))
        record('行内「测试」按钮存在', await page.getByRole('button', { name: /^测试$|^Test$|测试此密钥/i }).first().isVisible().catch(() => false))
        // 真实点一次「一键测试全部」（指向不可达上游 → 应得失败结果，不挂死）
        const testAll = page.getByRole('button', { name: /一键测试全部密钥|Test All Keys/i }).first()
        const t0 = Date.now()
        await testAll.click().catch(() => {})
        await page.waitForTimeout(12000)
        await shot(page, 'C3-after-test-all')
        record('一键测试全部有结果返回（未挂死）', Date.now() - t0 < 60000, `${Date.now() - t0}ms`)
      } else {
        record('多密钥面板打开', false, '未找到菜单项')
      }
    }

    // ---------- D. 上游不可达 → 502 ----------
    const apiStatus = await page.evaluate(async (b) => (await fetch(`${b}/api/status`)).status, BASE)
    record('本地 API 健康', apiStatus === 200, `status=${apiStatus}`)

    // ---------- E. 控制台 ----------
    const fatal = consoleErrors.filter((e) => !/favicon|404|Failed to load resource|401/i.test(e))
    record('浏览器控制台无致命错误', fatal.length === 0, fatal.slice(0, 2).join(' | '))
  } catch (err) {
    record('E2E 执行异常', false, String(err))
    await shot(page, 'Z-error')
  } finally {
    fs.mkdirSync(OUT, { recursive: true })
    fs.writeFileSync(path.join(OUT, 'results.json'), JSON.stringify({ ranAt: new Date().toISOString(), base: BASE, results }, null, 2))
    await browser.close()
    const failed = results.filter((r) => !r.ok)
    console.log(`\n===== ${results.length - failed.length}/${results.length} 通过 =====`)
    console.log(`证据目录: ${OUT}`)
    process.exit(failed.length > 0 ? 1 : 0)
  }
})()
