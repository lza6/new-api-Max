// 图片链路 E2E：用户附图渲染 + 下载按钮 + 放大 + 用作参考图。
// 用真实的本地图片（1x1 PNG）注入附件，断言用户可见的 DOM 行为。
const { chromium } = require('playwright')
const path = require('path')

const OUT = 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/计划书/e2e-evidence/playground'
const BASE = 'http://127.0.0.1:3005'

// 一张真实的小 PNG（2x2 红点），用于上传。
const TINY_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFElEQVR42mP8z8BQz0AEYBxVSF+FAP5FDvcfRYWgAAAAAElFTkSuQmCC',
  'base64'
)

;(async () => {
  const browser = await chromium.launch()
  const page = await (await browser.newContext({ viewport: { width: 1440, height: 900 } })).newPage()
  const errs = []
  page.on('pageerror', (e) => errs.push(String(e)))

  try {
    await page.goto(`${BASE}/login`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(2500)
    await page.fill('input[name="username"], input[type="text"]', 'root')
    await page.fill('input[type="password"]', 'TestPass123!')
    await page.keyboard.press('Enter')
    await page.waitForTimeout(4000)

    await page.goto(`${BASE}/playground`, { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(4000)

    // ---- 1. 通过隐藏 file input 注入一张真实图片 ----
    const fileInput = page.locator('input[type="file"]').first()
    const hasInput = (await fileInput.count()) > 0
    console.log(`  ${hasInput ? '✅' : '❌'} 存在文件输入: ${hasInput}`)
    if (!hasInput) throw new Error('找不到 file input')

    await fileInput.setInputFiles({ name: 'ref.png', mimeType: 'image/png', buffer: TINY_PNG })
    await page.waitForTimeout(2500)
    await page.screenshot({ path: path.join(OUT, '10-attachment-added.png') })

    // 附件缩略图出现（PromptInputAttachment 渲染 img）
    const attachImgs = await page.locator('form img, [data-slot] img').count()
    console.log(`  ${attachImgs > 0 ? '✅' : '❌'} 附件缩略图数量: ${attachImgs}`)

    // ---- 2. 发送（无文字仅有图也应可提交）----
    const sendBtn = page.getByRole('button', { name: /发送|Send/i }).first()
    const canSend = await sendBtn.isEnabled().catch(() => false)
    console.log(`  ${canSend ? '✅' : '❌'} 仅有图片时发送按钮可用: ${canSend}`)

    if (canSend) {
      await sendBtn.click()
      await page.waitForTimeout(12000) // 等真实上游返回
      await page.screenshot({ path: path.join(OUT, '11-after-send.png') })
    }

    // ---- 3. 对话里的图片：下载按钮 + 放大 ----
    const dlBtns = await page.getByLabel(/下载图片|Download image/).count()
    const viewBtns = await page.getByLabel(/查看图片|View image/).count()
    console.log(`  ${dlBtns > 0 ? '✅' : '❌'} 对话图片下载按钮数: ${dlBtns}`)
    console.log(`  ${viewBtns > 0 ? '✅' : '❌'} 对话图片放大按钮数: ${viewBtns}`)

    if (dlBtns > 0) {
      const dl = page.getByLabel(/下载图片|Download image/).first()
      const href = await dl.getAttribute('href')
      const name = await dl.getAttribute('download')
      console.log(`  ✅ 下载 href 前缀: ${(href || '').slice(0, 30)}…  download=${name}`)
    }

    if (viewBtns > 0) {
      await page.getByLabel(/查看图片|View image/).first().click()
      await page.waitForTimeout(1500)
      await page.screenshot({ path: path.join(OUT, '12-lightbox.png') })
      const preview = await page.getByText(/放大预览|Enlarged image preview/).count()
      console.log(`  ${preview > 0 ? '✅' : '❌'} 放大预览已打开: ${preview > 0}`)
      await page.keyboard.press('Escape')
      await page.waitForTimeout(800)
    }

    // ---- 4. 用作参考图 ----
    const refBtns = await page.getByLabel(/用作参考图|Use as reference image/).count()
    console.log(`  ${refBtns > 0 ? '✅' : '❌'} 「用作参考图」按钮数: ${refBtns}`)

    if (refBtns > 0) {
      const before = await page.locator('form img').count()
      await page.getByLabel(/用作参考图|Use as reference image/).first().click()
      await page.waitForTimeout(3000)
      const after = await page.locator('form img').count()
      console.log(`  ${after > before ? '✅' : '❌'} 点击后附件数 ${before} → ${after}`)
      await page.screenshot({ path: path.join(OUT, '13-reference-added.png') })
    }

    console.log(`\n  控制台错误: ${errs.length}`)
    errs.slice(0, 3).forEach((e) => console.log(`    ⚠️ ${e.slice(0, 140)}`))
  } catch (e) {
    console.log('  ❌ 异常:', e.message.split('\n')[0])
    await page.screenshot({ path: path.join(OUT, '99-error.png') })
  } finally {
    await browser.close()
  }
})()
