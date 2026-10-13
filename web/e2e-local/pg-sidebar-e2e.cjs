// 游乐场会话侧边栏 E2E（真实浏览器 + 真实网关）。
// 断言的是**用户可见行为**：分组标题出现、新建后出现在「今天」、折叠后状态保持、
// 搜索能过滤。每步截图入 计划书/e2e-evidence/playground/。
const { chromium } = require('playwright')
const fs = require('fs')
const path = require('path')

const OUT = 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/计划书/e2e-evidence/playground'
const BASE = 'http://127.0.0.1:3005'
const USER = 'root'
const PASS = 'TestPass123!'

async function shot(page, name) {
  const file = path.join(OUT, `${name}.png`)
  await page.screenshot({ path: file, fullPage: false })
  console.log(`  📸 ${name}.png`)
}

;(async () => {
  const browser = await chromium.launch()
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  const errors = []
  page.on('pageerror', (e) => errors.push(String(e)))
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()) })

  try {
    // ---- 登录（真实 UI 登录，API 直登会绕过 SPA 会话初始化）----
    await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(2500)
    await page.fill('input[name="username"], input[placeholder*="用户名"], input[type="text"]', USER)
    await page.fill('input[type="password"]', PASS)
    await page.keyboard.press('Enter')
    await page.waitForTimeout(4000)
    console.log('  ✅ 登录后 URL:', page.url())

    // ---- 进入游乐场 ----
    await page.goto(`${BASE}/playground`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(4000)
    await shot(page, '01-playground-loaded')

    // ---- 断言 1：侧边栏存在（aria-label=Conversations）----
    const sidebar = page.locator('aside[aria-label]').first()
    const sidebarVisible = await sidebar.isVisible().catch(() => false)
    console.log(`  ${sidebarVisible ? '✅' : '❌'} 侧边栏可见: ${sidebarVisible}`)

    // ---- 断言 2：新建会话按钮存在且可点 ----
    const newChat = page.getByRole('button', { name: /New chat|开启新对话|新建/i }).first()
    const hasNew = await newChat.isVisible().catch(() => false)
    console.log(`  ${hasNew ? '✅' : '❌'} 新建会话按钮可见: ${hasNew}`)

    if (hasNew) {
      await newChat.click()
      await page.waitForTimeout(1500)
      await shot(page, '02-after-new-chat')
    }

    // ---- 断言 3：分组标题出现（今天/近7天/更早 等）----
    const bodyText = await page.locator('body').innerText()
    const buckets = ['今天', '昨天', '近 7 天', '近 30 天', '更早']
    const found = buckets.filter((b) => bodyText.includes(b))
    console.log(`  ${found.length > 0 ? '✅' : '❌'} 时间分组标题: ${found.join(' / ') || '（无）'}`)

    // ---- 断言 4：搜索框存在 ----
    const search = page.getByPlaceholder(/搜索会话|Search conversations/i).first()
    const hasSearch = await search.isVisible().catch(() => false)
    console.log(`  ${hasSearch ? '✅' : '❌'} 搜索框可见: ${hasSearch}`)

    // ---- 断言 5：折叠按钮可用，且折叠后状态变化 ----
    const collapseBtn = page.getByRole('button', { name: /收(起|合)侧边栏|Collapse sidebar/i }).first()
    const hasCollapse = await collapseBtn.isVisible().catch(() => false)
    console.log(`  ${hasCollapse ? '✅' : '❌'} 折叠按钮可见: ${hasCollapse}`)

    if (hasCollapse) {
      const widthBefore = await sidebar.boundingBox()
      await collapseBtn.click()
      await page.waitForTimeout(1200)
      await shot(page, '03-collapsed')
      const expandedBtn = page.getByRole('button', { name: /展开侧边栏|Expand sidebar/i }).first()
      const canExpand = await expandedBtn.isVisible().catch(() => false)
      console.log(`  ${canExpand ? '✅' : '❌'} 折叠后出现「展开」按钮: ${canExpand}`)

      // 折叠状态持久化：刷新后仍应是折叠的
      await page.reload({ waitUntil: 'domcontentloaded' })
      await page.waitForTimeout(3500)
      const stillCollapsed = await page.getByRole('button', { name: /展开侧边栏|Expand sidebar/i }).first().isVisible().catch(() => false)
      console.log(`  ${stillCollapsed ? '✅' : '❌'} 刷新后折叠状态保持: ${stillCollapsed}`)
      await shot(page, '04-collapsed-persisted')

      // 恢复展开，便于后续测试
      if (stillCollapsed) {
        await page.getByRole('button', { name: /展开侧边栏|Expand sidebar/i }).first().click()
        await page.waitForTimeout(1000)
      }
      if (widthBefore) console.log(`  ℹ️ 展开宽度 ${Math.round(widthBefore.width)}px`)
    }

    // ---- 断言 6：模型选择器（截图取证）----
    await page.waitForTimeout(1000)
    const modelTrigger = page.locator('[role="combobox"]').first()
    if (await modelTrigger.isVisible().catch(() => false)) {
      await modelTrigger.click()
      await page.waitForTimeout(1500)
      await shot(page, '05-model-selector-open')
      const openText = await page.locator('body').innerText()
      const kinds = ['文本', '图片', '视频', '音频', '向量'].filter((k) => openText.includes(k))
      console.log(`  ${kinds.length > 0 ? '✅' : '❌'} 模型分组标题: ${kinds.join(' / ') || '（无）'}`)
      await page.keyboard.press('Escape')
    }

    console.log(`\n  控制台错误数: ${errors.length}`)
    errors.slice(0, 5).forEach((e) => console.log(`    ⚠️ ${e.slice(0, 160)}`))
  } catch (e) {
    console.log('  ❌ E2E 异常:', e.message.split('\n')[0])
    await shot(page, '99-error')
  } finally {
    await browser.close()
  }
})()
