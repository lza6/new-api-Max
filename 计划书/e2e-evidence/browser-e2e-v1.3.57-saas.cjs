// new-api v1.3.57 浏览器 E2E：套餐对比页 / 定价页余额 / 续费按钮（playwright-core + 系统 Chrome）
// 依赖：本地服务已起（BASE 默认 http://127.0.0.1:3000，可改环境变量 BASE）
// 用法：node browser-e2e-v1.3.57-saas.cjs
const { chromium } = require('C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/browser-e2e/node_modules/playwright-core');
const fs = require('fs');
const path = require('path');

const BASE = process.env.BASE || 'http://127.0.0.1:3000';
const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const OUT_DIR = path.resolve('计划书/e2e-evidence/browser-e2e-v1.3.57-saas');
const ROOT = { username: 'root', password: 'Admin-2026!' };
const VIEWPORTS = [
  { name: '375', width: 375, height: 720 },
  { name: '768', width: 768, height: 900 },
  { name: '1280', width: 1280, height: 860 },
];

const results = [];
function record(name, ok, detail = '') {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' :: ' + detail : ''}`);
}
async function shot(page, name) {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  await page.screenshot({ path: path.join(OUT_DIR, name) });
}
async function assertNoHOverflow(page, label, vp) {
  const sw = await page.evaluate(() => document.documentElement.scrollWidth);
  const bodySw = await page.evaluate(() => document.body.scrollWidth);
  const ok = sw <= vp.width + 1 && bodySw <= vp.width + 1;
  record(`[${vp.name}] ${label} 无横向溢出`, ok, `docSW=${sw} bodySW=${bodySw}`);
  return ok;
}

(async () => {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  const browser = await chromium.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ locale: 'en-US' });
  const page = await context.newPage();
  page.setDefaultTimeout(45000);

  // ---------- 前置：登录（建立 dashboard session）→ compliance + 建套餐 + 绑定 ----------
  let complianceOk = false, plansCreated = 0, subBound = false;
  try {
    // setup 根管理员（已初始化则失败忽略）
    await page.request
      .post(`${BASE}/api/setup`, { data: { username: ROOT.username, password: ROOT.password, confirmPassword: ROOT.password } })
      .catch(() => {});
    // UI 登录（拿 dashboard session，compliance 端点拒绝 API token）
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2000);
    const hasInput = await page.locator('input[name="username"]').count();
    if (hasInput > 0) {
      await page.fill('input[name="username"]', ROOT.username);
      await page.fill('input[name="password"]', ROOT.password);
      await page.locator('button[type="submit"]').first().click();
      await page.waitForTimeout(4500);
    }
    // 通过 UI 登录拿 dashboard access token，再带 Bearer fetch 完成前置
    const loginResp = await page.request.post(`${BASE}/api/user/login`, { data: { username: ROOT.username, password: ROOT.password } });
    const loginTok = (await loginResp.json())?.data?.access_token || '';
    const setup = await page.evaluate(async ({ base, tok }) => {
      const out = { compliance: null, plans: [], bind: null, err: '' };
      const H = { 'Content-Type': 'application/json', Authorization: 'Bearer ' + tok };
      try {
        out.compliance = await fetch(`${base}/api/option/payment_compliance`, {
          method: 'POST', headers: H, body: JSON.stringify({ confirmed: true }),
        }).then((r) => r.json());
        for (const p of [
          { title: 'Free', subtitle: 'Starter plan', price_amount: 0, duration_unit: 'month', duration_value: 1, total_amount: 100000, concurrency_limit: 2, rpm_limit: 30, sort_order: 1 },
          { title: 'Pro', subtitle: 'Power plan', price_amount: 29, duration_unit: 'month', duration_value: 1, total_amount: 1000000, concurrency_limit: 16, rpm_limit: 600, sort_order: 2 },
        ]) {
          out.plans.push(await fetch(`${base}/api/subscription/admin/plans`, {
            method: 'POST', headers: H, body: JSON.stringify({ plan: p }),
          }).then((r) => r.json()));
        }
        const list = await fetch(`${base}/api/subscription/admin/plans`, { headers: H }).then((r) => r.json());
        const pro = (list?.data || []).find((x) => x?.plan?.title === 'Pro');
        if (pro) {
          out.bind = await fetch(`${base}/api/subscription/admin/bind`, {
            method: 'POST', headers: H, body: JSON.stringify({ user_id: 1, plan_id: pro.plan.id }),
          }).then((r) => r.json());
        }
      } catch (e) { out.err = String(e).slice(0, 150); }
      return out;
    }, { base: BASE, tok: loginTok });
    complianceOk = setup.compliance?.success !== false;
    plansCreated = setup.plans.filter((p) => p?.success !== false).length;
    subBound = setup.bind?.success !== false && setup.bind != null;
    console.log('前置结果:', JSON.stringify({
      compliance: Boolean(setup.compliance?.success), plans: setup.plans.map((p) => p?.success),
      bind: setup.bind?.success ?? null, err: setup.err,
    }).slice(0, 220));
  } catch (e) { console.log('setup warn:', String(e).slice(0, 120)); }
  record('E2E 前置：compliance+套餐+绑定', complianceOk && plansCreated === 2, `plans=${plansCreated} bound=${subBound}`);

  // ---------- 三断点截图（复用同一登录 context，setViewportSize 切断点） ----------
  for (const vp of VIEWPORTS) {
    await page.setViewportSize({ width: vp.width, height: vp.height });

    // 1) 定价页（余额 banner + 套餐对比入口）
    await page.goto(`${BASE}/pricing`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(4500);
    const compareLink = await page.locator(`a[href="/pricing/plans"]`).count();
    record(`[${vp.name}] 定价页「套餐对比」入口`, compareLink > 0, `link=${compareLink}`);
    await shot(page, `${vp.name}-pricing-balance.png`);

    // 2) 套餐对比页
    await page.goto(`${BASE}/pricing/plans`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(6000);
    const table = await page.locator('table, [role="table"]').count();
    record(`[${vp.name}] 套餐对比页表格渲染`, table > 0, `table=${table}`);
    await assertNoHOverflow(page, '套餐对比页', vp);
    await shot(page, `${vp.name}-plan-compare.png`);

    // 3) 续费按钮（Pro 行「立即续费」）
    const renewBtnText = await page.getByText(/renew/i).count();
    const subscribeBtn = await page.getByRole('button', { name: /subscribe|renew/i }).count();
    record(`[${vp.name}] 续费/订阅按钮`, renewBtnText > 0 || subscribeBtn > 0, `renewText=${renewBtnText} btn=${subscribeBtn}`);
    if (renewBtnText > 0 || subscribeBtn > 0) {
      await shot(page, `${vp.name}-renew-cta.png`);
    }
  }

  await browser.close();
  const passed = results.filter((r) => r.ok).length;
  const failed = results.filter((r) => !r.ok).length;
  console.log(`\n==== 汇总: ${passed} PASS / ${failed} FAIL / ${results.length} total ====`);
  fs.writeFileSync(path.join(OUT_DIR, 'summary.json'), JSON.stringify(results, null, 2));
  process.exit(failed > 0 ? 1 : 0);
})().catch((e) => { console.error('E2E ERROR:', e.message); process.exit(1); });