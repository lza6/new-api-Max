// 运营面板「利润与成本 + 成本设置」真实浏览器验证。
const { chromium } = require('../web/node_modules/playwright')
const BASE = process.argv[2] || 'http://127.0.0.1:18123'
let pass = 0, fail = 0
function check(name, cond, detail) { if (cond) { pass++; console.log('  ✅ '+name) } else { fail++; console.log('  ❌ '+name+(detail?' — '+detail:'')) } }
async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE })
  const login = await context.request.post('/api/user/login', { data: { username: 'smoke', password: 'SmokeTest123456' } })
  const lj = await login.json().catch(() => null)
  check('API 登录成功', lj?.success === true)
  const page = await context.newPage()
  await page.goto('/ops-panel', { waitUntil: 'networkidle' })
  await page.waitForTimeout(2500)
  const body = (await page.locator('body').textContent().catch(() => '')) || ''
  check('利润与成本区块渲染', /利润与成本|Profit & Cost/.test(body), body.slice(0, 300))
  check('本月/本年财务数字渲染', /本月|This Month/.test(body) && /本年|This Year/.test(body))
  check('预计月度/年度利润渲染', /预计本月利润|Projected Monthly Profit/.test(body) && /预计本年利润|Projected Yearly Profit/.test(body))
  check('建议每日上游充值渲染', /建议每日上游充值|Recommended Daily Upstream Top-up/.test(body))
  check('模型成本表渲染', /模型成本|Model Costs/.test(body))
  for (const m of ['deepseek-v4.1-flash','step-5-preview','gpt-6-luna','glm-5.3-flash']) {
    check('成本规则可见: '+m, body.includes(m))
  }
  check('成本规则模式为按次', /按次|Per call/.test(body))
  await page.screenshot({ path: '计划书/e2e-evidence/v13158-ops-profit.png', fullPage: true })
  await browser.close()
  console.log(`\n=== 结果：${pass} 通过 / ${fail} 失败 ===`)
  process.exit(fail === 0 ? 0 : 1)
}
main().catch((e) => { console.error(e); process.exit(1) })
