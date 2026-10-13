// T10/T9 E2E：批量生成张数控件的门控。
//
// 关键：**显式设置模型**再断言，不依赖 localStorage 里的历史值——
// 首版用例假定页面是文本模型，但上一次运行已把 dall-e-3 持久化，
// 于是「文本模型下无控件」这条断言假失败（代码是对的，用例错了）。
const { chromium } = require('playwright')
const path = require('path')

const OUT = 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/计划书/e2e-evidence/playground'
const BASE = 'http://127.0.0.1:3005'
const TEXT_MODEL = 'deepseek-v4.1-flash'
const IMAGE_MODEL = 'dall-e-3'

async function openPlayground(page, model) {
  await page.goto(`${BASE}/playground`, { waitUntil: 'domcontentloaded' })
  // 先重置持久化配置，确保起点确定。
  await page.evaluate((m) => {
    localStorage.setItem(
      'playground_config',
      JSON.stringify({
        version: 1,
        data: {
          model: m, group: 'default', temperature: 0.7, top_p: 1, max_tokens: 4096,
          frequency_penalty: 0, presence_penalty: 0, seed: null, stream: true,
          size: '', batch_count: 1, reasoning_effort: '', system_prompt: '',
        },
      })
    )
    localStorage.setItem(
      'playground_parameter_enabled',
      JSON.stringify({ version: 1, data: { temperature: true, top_p: true, max_tokens: false, frequency_penalty: true, presence_penalty: true, seed: false, size: true, batch_count: true, reasoning_effort: false } })
    )
  }, model)
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)
}

async function openParams(page) {
  const btn = page.getByRole('button', { name: /参数|Parameters/i }).first()
  if (await btn.isVisible().catch(() => false)) {
    await btn.click()
    await page.waitForTimeout(1200)
  }
}

const hasBatch = async (page) =>
  /每次生成张数|Images per request/.test(await page.locator('body').innerText())

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

    // ---- 文本模型：不应有张数控件 ----
    await openPlayground(page, TEXT_MODEL)
    await openParams(page)
    const textHas = await hasBatch(page)
    console.log(`  ${!textHas ? '✅' : '❌'} 文本模型(${TEXT_MODEL})下无张数控件: ${!textHas}`)
    await page.screenshot({ path: path.join(OUT, '20-text-model-params.png') })

    // ---- 生图模型：应有张数控件 + 明示上限 ----
    await openPlayground(page, IMAGE_MODEL)
    await openParams(page)
    const imgHas = await hasBatch(page)
    console.log(`  ${imgHas ? '✅' : '❌'} 生图模型(${IMAGE_MODEL})下出现张数控件: ${imgHas}`)
    const body = await page.locator('body').innerText()
    const showsMax = /上限 8|max 8/.test(body)
    console.log(`  ${showsMax ? '✅' : '❌'} 明示上限 8 张（计费提示）: ${showsMax}`)
    const hasSize = /图片尺寸|Image Size/.test(body)
    console.log(`  ${hasSize ? '✅' : '❌'} 同时显示图片尺寸控件: ${hasSize}`)
    await page.screenshot({ path: path.join(OUT, '21-image-model-batch.png') })

    console.log(`\n  控制台错误: ${errs.length}`)
    errs.slice(0, 3).forEach((e) => console.log(`    ⚠️ ${e.slice(0, 140)}`))
  } catch (e) {
    console.log('  ❌ 异常:', e.message.split('\n')[0])
    await page.screenshot({ path: path.join(OUT, '99-error.png') })
  } finally {
    await browser.close()
  }
})()
