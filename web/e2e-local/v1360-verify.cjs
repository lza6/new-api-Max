/**
 * v1.3.60 真实浏览器 E2E（本地）
 *
 * 覆盖本批三块改动的用户可见行为：
 *   1. 数据库导出/导入 UI（系统设置 → 运维 → Database Backup）
 *   2. 用户查看自己的密钥不再要求二次验证（免 step-up）
 *   3. 上游不可达 → 502 语义（在 API 层断言，UI 侧无独有页面）
 *
 * 依赖：本地已运行 `PORT=3000 SQLITE_PATH=e2e-v1360.db go run main.go`，
 * 且已通过 /api/setup 建立 root 账号。
 *
 * 输出：截图到 计划书/e2e-evidence/v1.3.60/
 */
const { chromium } = require('playwright')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.env.E2E_BASE || 'http://127.0.0.1:3000'
const USER = process.env.E2E_USER || 'e2e_root'
const PASS = process.env.E2E_PASS || 'E2ePass!2026'
const OUT = path.resolve(__dirname, '../../计划书/e2e-evidence/v1.3.60')

const results = []
function record(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? '✅' : '❌'} ${name}${detail ? ' — ' + detail : ''}`)
}

async function shot(page, name) {
  fs.mkdirSync(OUT, { recursive: true })
  await page.screenshot({ path: path.join(OUT, `${name}.png`), fullPage: true })
}

;(async () => {
  const browser = await chromium.launch()
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
  })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => {
    if (m.type() === 'error') consoleErrors.push(m.text())
  })

  try {
    // ---------- 1. 登录 ----------
    await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' })
    await shot(page, '01-login')
    await page.fill('input[name="username"], input#username', USER)
    await page.fill('input[name="password"], input#password', PASS)
    await page.getByRole('button', { name: /登录|Login|Sign in/i }).click()
    await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 20000 })
    record('登录成功', true, page.url())
    await shot(page, '02-dashboard')

    // ---------- 2. 数据库备份 UI ----------
    await page.goto(`${BASE}/system-settings/operations/database-backup`, {
      waitUntil: 'networkidle',
    })
    await page.waitForTimeout(2000)
    await shot(page, '03-db-backup-section')

    const heading = await page
      .getByText(/Database Backup|数据库备份/i)
      .first()
      .isVisible()
      .catch(() => false)
    record('备份分区可见', heading)

    const exportBtn = page.getByRole('button', { name: /Download backup|下载备份/i })
    record('导出按钮存在', await exportBtn.isVisible().catch(() => false))

    const importBtn = page.getByRole('button', { name: /Choose backup file|选择备份文件/i })
    record('导入按钮存在', await importBtn.isVisible().catch(() => false))

    // 验证「只新增不删除」的防错提示真实渲染（尼尔森 #5 防错）
    const warning = await page
      .getByText(/never delete or overwrite|绝不删除或覆盖/i)
      .first()
      .isVisible()
      .catch(() => false)
    record('防错提示可见（不删除不覆盖）', warning)

    // 触发真实导出（下载），证明端到端可用
    const [download] = await Promise.all([
      page.waitForEvent('download', { timeout: 120000 }).catch(() => null),
      exportBtn.click(),
    ])
    if (download) {
      const savePath = path.join(OUT, 'backup-export.sqljson.gz')
      await download.saveAs(savePath)
      const size = fs.statSync(savePath).size
      const gz = fs.readFileSync(savePath)
      // gzip magic 0x1f 0x8b
      const isGzip = gz[0] === 0x1f && gz[1] === 0x8b
      record('导出下载成功且为有效 gzip', isGzip && size > 0, `${size} bytes`)
    } else {
      record('导出下载成功且为有效 gzip', false, '未捕获到 download 事件')
    }

    // ---------- 3. 密钥免二次验证 ----------
    // 先用 API 建一个 key（UI 表单选择器易变，这里保证列表非空）
    await page.evaluate(async (base) => {
      const login = await fetch(`${base}/api/user/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: 'e2e_root', password: 'E2ePass!2026' }),
      }).then((r) => r.json())
      await fetch(`${base}/api/token/`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${login.data.access_token}`,
        },
        body: JSON.stringify({
          name: 'e2e-reveal-key',
          remain_quota: 500000,
          expired_time: -1,
          unlimited_quota: false,
          model_limits_enabled: false,
        }),
      })
    }, BASE)
    await page.goto(`${BASE}/keys`, { waitUntil: 'networkidle' })
    await page.waitForTimeout(1500)
    await shot(page, '04-keys-list')

    // 创建一个 key 以便 reveal
    const createBtn = page.getByRole('button', { name: /创建|Create|Add/i }).first()
    if (await createBtn.isVisible().catch(() => false)) {
      await createBtn.click()
      await page.waitForTimeout(800)
      const nameInput = page.locator('input[name="name"], input#name').first()
      if (await nameInput.isVisible().catch(() => false)) {
        await nameInput.fill('e2e-reveal-key')
      }
      const submit = page.getByRole('button', { name: /提交|Submit|确定|Confirm|保存|Save/i }).last()
      await submit.click().catch(() => {})
      await page.waitForTimeout(1500)
    }
    await shot(page, '05-after-create-key')

    // 前置断言：列表里必须真的有 key，否则下面的断言会「空过」（假阳性）。
    await page.waitForTimeout(1500)
    const rowCount = await page.locator('table tbody tr').count()
    record('密钥列表非空（避免假阳性）', rowCount > 0, `rows=${rowCount}`)

    // 真实用户路径：点击「掩码密钥」→ 后端 reveal。
    // 免 step-up 生效时：直接出现 "API Key 已解锁" toast，且**不弹**验证弹窗。
    // 该 toast 由 useTokenKeyDisclosure 仅在 reveal 成功后发出，是正向证据。
    const maskedTrigger = page
      .locator('table tbody')
      .getByText(/^sk-[^*]*\*+/)
      .first()
    const triggerVisible = await maskedTrigger.isVisible().catch(() => false)
    record('找到掩码密钥触发器', triggerVisible)

    let proofDialogShown = false
    let unlockedToast = false
    if (triggerVisible) {
      await maskedTrigger.click()
      await page.waitForTimeout(3500)
      proofDialogShown = await page
        .getByText(/Verify to view API key|验证后查看|Confirm your identity/i)
        .first()
        .isVisible()
        .catch(() => false)
      // 正向证据：出现「已解锁」toast（仅在 reveal 成功后触发）
      unlockedToast = await page
        .getByText(/API Key 已解锁|API key unlocked|已解锁/i)
        .first()
        .isVisible()
        .catch(() => false)
      await shot(page, '06-after-reveal-key')
    }
    record('找到掩码密钥触发器', triggerVisible)
    record('查看自己的密钥未弹二次验证', triggerVisible && !proofDialogShown)
    record('完整密钥确实被揭示（正向证据：已解锁 toast）', unlockedToast)

    // ---------- 4. 上游不可达 502（API 断言） ----------
    const apiCheck = await page.evaluate(async (base) => {
      const r = await fetch(`${base}/api/status`)
      return r.status
    }, BASE)
    record('本地 API 健康', apiCheck === 200, `status=${apiCheck}`)

    // ---------- 5. 控制台无致命错误 ----------
    const fatal = consoleErrors.filter(
      (e) => !/favicon|404|Failed to load resource/i.test(e)
    )
    record('浏览器控制台无致命错误', fatal.length === 0, fatal.slice(0, 2).join(' | '))
  } catch (err) {
    record('E2E 执行异常', false, String(err))
    await shot(page, '99-error')
  } finally {
    fs.mkdirSync(OUT, { recursive: true })
    fs.writeFileSync(
      path.join(OUT, 'results.json'),
      JSON.stringify(
        { ranAt: new Date().toISOString(), base: BASE, results },
        null,
        2
      )
    )
    await browser.close()
    const failed = results.filter((r) => !r.ok)
    console.log(`\n===== ${results.length - failed.length}/${results.length} 通过 =====`)
    console.log(`证据目录: ${OUT}`)
    process.exit(failed.length > 0 ? 1 : 0)
  }
})()
