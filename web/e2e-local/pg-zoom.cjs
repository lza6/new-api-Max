const { chromium } = require('playwright')
;(async () => {
  const b = await chromium.launch()
  const p = await (await b.newContext({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 })).newPage()
  await p.goto('http://127.0.0.1:3005/login', { waitUntil: 'domcontentloaded' })
  await p.waitForTimeout(2500)
  await p.fill('input[name="username"], input[type="text"]', 'root')
  await p.fill('input[type="password"]', 'TestPass123!')
  await p.keyboard.press('Enter')
  await p.waitForTimeout(4000)
  await p.goto('http://127.0.0.1:3005/playground', { waitUntil: 'domcontentloaded' })
  await p.waitForTimeout(4000)
  // 只截侧边栏顶部区域，放大看图标
  await p.screenshot({ path: 'C:/Users/Administrator.DESKTOP-EGNE9ND/Desktop/公益服务网关/new-api/计划书/e2e-evidence/playground/06-sidebar-zoom.png', clip: { x: 210, y: 290, width: 260, height: 160 } })
  // 顺便 dump 该区域每个按钮的可访问名与是否含 svg
  const info = await p.evaluate(() => {
    const aside = document.querySelector('aside[aria-label]')
    if (!aside) return 'no aside'
    return [...aside.querySelectorAll('button')].slice(0, 5).map((b) => ({
      label: b.getAttribute('aria-label') || b.textContent.trim().slice(0, 20),
      hasSvg: !!b.querySelector('svg'),
      size: b.getBoundingClientRect().width + 'x' + b.getBoundingClientRect().height,
    }))
  })
  console.log(JSON.stringify(info, null, 1))
  await b.close()
})()
