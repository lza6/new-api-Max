// new-api v1.3.46 浏览器 E2E（playwright-core + 系统 Chrome）— 强制 en-US 断言
const { chromium } = require('C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/browser-e2e/node_modules/playwright-core');
const fs = require('fs');
const path = require('path');

const BASE = 'http://127.0.0.1:18101';
const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const SHOT_DIR = 'C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/browser-e2e-shots';
const OUT_DIR = path.resolve('计划书/e2e-evidence/browser-e2e-v1.3.46');
const ROOT = { username: 'root', password: 'Admin-2026!' };

const results = [];
function record(name, ok, detail = '') {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' :: ' + detail : ''}`);
}
async function shot(page, name) {
  fs.mkdirSync(SHOT_DIR, { recursive: true });
  await page.screenshot({ path: path.join(SHOT_DIR, name) });
}

(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1360, height: 860 }, locale: 'en-US' });
  const page = await context.newPage();
  page.setDefaultTimeout(45000);

  try {
    // 0) Setup 根管理员（确认密码）
    const setup = await page.request.post(`${BASE}/api/setup`, { data: { username: ROOT.username, password: ROOT.password, confirmPassword: ROOT.password } });
    const setupBody = await setup.json();
    record('setup 根管理员', setup.ok() && setupBody.success !== false, JSON.stringify(setupBody).slice(0, 100));

    // 1) 公开定价页 + 订阅统计卡
    await page.goto(`${BASE}/pricing`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('h1', { timeout: 45000 }).catch(() => {});
    await page.waitForTimeout(6000);
    const statsCount = await page.locator('text=Active subscriptions').count();
    record('pricing 渲染订阅统计卡', statsCount > 0, `statsCard=${statsCount}`);
    await shot(page, '01-pricing-subscription-stats.png');

    // 2) 登录（真实表单）→ dashboard
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[name="username"]', { timeout: 45000 });
    await page.fill('input[name="username"]', ROOT.username);
    await page.fill('input[name="password"]', ROOT.password);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForTimeout(5000);
    record('登录跳离 /sign-in', !page.url().includes('/sign-in'), page.url());

    // 3) 创建 root 的 token（用登录 API 的 access_token）
    const login = await page.request.post(`${BASE}/api/user/login`, { data: { username: ROOT.username, password: ROOT.password } });
    const loginBody = await login.json();
    const accessToken = loginBody?.data?.access_token || '';
    const tok = await page.request.post(`${BASE}/api/token/`, {
      headers: accessToken ? { Authorization: 'Bearer ' + accessToken } : {},
      data: { name: 'browser-e2e-token', remain_quota: 500000, expired_time: -1, unlimited_quota: false, model_limits_enabled: false },
    });
    const tokBody = await tok.json();
    record('创建测试 token', tok.ok() && tokBody.success !== false, `id=${tokBody?.data?.id ?? tokBody?.message ?? ''}`);

    // 4) keys 页 step-up 弹窗（真实点击）
    await page.goto(`${BASE}/keys`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(5000);
    const masked = page.locator('span:text-matches("^sk-")').first();
    const maskedCount = await masked.count();
    if (maskedCount > 0) {
      await masked.click();
      await page.waitForTimeout(2500);
      const dlg = await page.locator('text=Verify to view API key').count();
      record('点击掩码 key → step-up 弹窗', dlg > 0, `dialog=${dlg}`);
      await shot(page, '03-keys-stepup-dialog.png');
      const pw = page.locator('input[type="password"]').first();
      if ((await pw.count()) > 0) {
        await pw.fill(ROOT.password);
        await page.locator('button:has-text("Verify")').first().click();
        await page.waitForTimeout(3000);
        const unlocked = await page.locator('text=API key unlocked').count();
        record('密码验证 → key 披露', unlocked > 0, `unlocked=${unlocked}`);
        await shot(page, '04-keys-stepup-unlocked.png');
      }
    } else {
      record('keys 页存在掩码 key 单元格', false, 'no sk- cell');
    }

    // 5) webhook 管理页（admin，en-US 文案）
    await page.goto(`${BASE}/webhook`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(4000);
    const whTitle = await page.locator('text=Webhook Notifications').count();
    const whUrl = await page.locator('input#webhook-url').count();
    record('webhook 设置页渲染', whTitle > 0 && whUrl > 0, `title=${whTitle} urlInput=${whUrl}`);
    await shot(page, '02-webhook-settings.png');
  } catch (e) {
    record('脚本异常', false, String(e).slice(0, 300));
    await shot(page, '99-error.png').catch(() => {});
  }

  fs.mkdirSync(OUT_DIR, { recursive: true });
  fs.writeFileSync(path.join(OUT_DIR, 'results.json'), JSON.stringify({ base: BASE, total: results.length, passed: results.filter(r => r.ok).length, results }, null, 2));
  if (fs.existsSync(SHOT_DIR)) {
    for (const f of fs.readdirSync(SHOT_DIR)) fs.copyFileSync(path.join(SHOT_DIR, f), path.join(OUT_DIR, f));
  }
  console.log(`\n===== ${results.filter(r => r.ok).length}/${results.length} PASS =====`);
  await browser.close();
  process.exit(results.every(r => r.ok) ? 0 : 1);
})();
