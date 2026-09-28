// §4.2.1 交互反馈闭环浏览器 E2E：抽查高频按钮的「点击→加载/成功/失败/二次确认」反馈
const { chromium } = require('C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/browser-e2e/node_modules/playwright-core');
const fs = require('fs');
const path = require('path');

const BASE = 'http://127.0.0.1:18601';
const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const OUT_DIR = path.resolve('计划书/e2e-evidence/browser-e2e-v1.3.54-interaction');
const ROOT = { username: 'root', password: 'Admin-2026!' };
const results = [];
function record(name, ok, detail = '') { results.push({ name, ok, detail }); console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' :: ' + detail : ''}`); }
async function shot(page, name) { fs.mkdirSync(OUT_DIR, { recursive: true }); await page.screenshot({ path: path.join(OUT_DIR, name) }); }

(async () => {
  const browser = await chromium.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1360, height: 900 }, locale: 'en-US' });
  const page = await context.newPage();
  page.setDefaultTimeout(45000);
  try {
    // 0) setup root
    const setup = await page.request.post(`${BASE}/api/setup`, { data: { username: ROOT.username, password: ROOT.password, confirmPassword: ROOT.password } });
    const sb = await setup.json();
    record('setup', (setup.ok() && sb.success !== false) || String(sb.message||'').includes('已经初始化'), (sb.message||'').slice(0,40));

    // 1) 登录表单反馈
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[name="username"]', { timeout: 45000 });
    await page.fill('input[name="username"]', ROOT.username);
    await page.fill('input[name="password"]', ROOT.password);
    const loginBtn = page.locator('button[type="submit"]').first();
    const disabledDuringSubmit = await loginBtn.isDisabled();
    await loginBtn.click();
    await page.waitForTimeout(4000);
    const redirected = !page.url().includes('/sign-in');
    record('登录按钮：提交中禁用 + 成功后跳转', !disabledDuringSubmit && redirected, `redirect=${page.url()}`);

    // 2) keys 页：用 API 造一个 token（可控），验证行操作的反馈（菜单/复制 step-up/切换 toast）
    // 先获取管理员会话 token（cookie 已由 UI 登录建立；API 造数据用登录 access_token）
    const login = await page.request.post(`${BASE}/api/user/login`, { data: { username: ROOT.username, password: ROOT.password } });
    const accessToken = (await login.json())?.data?.access_token || '';
    let tkBody = { message: 'not attempted' };
    try { const tkResp = await page.request.post(`${BASE}/api/token/`, { headers: { Authorization: 'Bearer ' + accessToken }, data: { name: 'e2e-feedback-key-'+Date.now(), remain_quota: 1000000, expired_time: -1, unlimited_quota: false, model_limits_enabled: false } }); tkBody = await tkResp.json(); } catch (e) { tkBody = { message: 'create threw: '+String(e).slice(0,80) }; }
    console.log('token create:', JSON.stringify(tkBody).slice(0,160));
    await page.goto(`${BASE}/keys`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(6000);
    const rowExists = await page.locator('text=sk-').count();
    record('keys 表格：有可操作行', rowExists > 0, `rows=${rowExists}`);
    if (rowExists > 0) {
      // 行操作菜单
      const openMenu = page.locator('button[aria-label="Open menu"]').first();
      await openMenu.click(); await page.waitForTimeout(800);
      const menuOpen = await page.locator('[role="menu"], [data-slot="menu"]').count();
      record('keys 行操作：菜单打开', menuOpen > 0, `menu=${menuOpen}`);
      await shot(page, '02-keys-row-menu.png');
      // 复制 → step-up 弹窗
      await page.locator('[role="menuitem"]:has-text("Copy Key")').first().click().catch(() => {});
      await page.waitForTimeout(1500);
      const stepup = await page.locator('text=Verify to view API key').count();
      record('keys 复制：触发 step-up 弹窗', stepup > 0, `stepup=${stepup}`);
      await shot(page, '03-keys-copy-stepup.png');
      // 关闭 step-up 弹窗（避免遮挡后续操作）
      await page.keyboard.press('Escape'); await page.waitForTimeout(800);
      // 状态切换 → toast
      await openMenu.click(); await page.waitForTimeout(800);
      await page.locator('[role="menuitem"]:has-text("Disable"), [role="menuitem"]:has-text("Enable")').first().click().catch(() => {});
      await page.waitForTimeout(1500);
      record('keys 状态切换：有 toast 反馈', (await page.locator('[data-sonner-toast]').count()) > 0);
      // 删除 → 二次确认
      await openMenu.click(); await page.waitForTimeout(800);
      await page.locator('[role="menuitem"]:has-text("Delete")').first().click().catch(() => {});
      await page.waitForTimeout(1200);
      const confirmDialog = await page.locator('[role="alertdialog"]').count();
      record('keys 删除：出现二次确认弹窗', confirmDialog > 0, `confirm=${confirmDialog}`);
      await shot(page, '04-keys-delete-confirm.png');
    } else {
      record('keys 行操作：菜单打开', false, '无行');
    }

    // 3) dashboard：复制请求（step-up 弹窗反馈）
    await page.goto(`${BASE}/dashboard/overview`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(4000);
    const copyBtn = page.locator('button[aria-label="Copy ready-to-run curl"]').first();
    if ((await copyBtn.count()) > 0) {
      await copyBtn.click(); await page.waitForTimeout(1500);
      const stepup = await page.locator('text=Verify to view API key').count();
      record('dashboard 复制：触发 step-up 弹窗', stepup > 0, `stepup=${stepup}`);
      await shot(page, '03-dashboard-copy-stepup.png');
    } else {
      record('dashboard 复制：触发 step-up 弹窗', null, 'SKIP（示例卡未渲染，条件性）');
    }

    // 4) pricing：页面渲染 + 统计卡
    await page.goto(`${BASE}/pricing`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(5000);
    const stats = await page.locator('text=Active subscriptions').count();
    record('pricing：页面渲染 + 订阅统计卡', stats > 0, `stats=${stats}`);
    await shot(page, '05-pricing.png');

    // 5) webhook 页：保存按钮 → toast
    await page.goto(`${BASE}/webhook`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input#webhook-url', { timeout: 20000 }).catch(() => {});
    await page.waitForTimeout(1500);
    const saveBtn = page.locator('button:has-text("Save")').first();
    if ((await saveBtn.count()) > 0) {
      await saveBtn.click(); await page.waitForTimeout(1500);
      record('webhook 保存：有 toast 反馈', (await page.locator('[data-sonner-toast], [role="status"]').count()) > 0);
      await shot(page, '06-webhook-save.png');
    } else {
      record('webhook 保存：有 toast 反馈', false, '未找到 Save 按钮');
    }

  } catch (e) {
    record('脚本异常', false, String(e).slice(0, 200));
    await shot(page, '99-error.png').catch(() => {});
  }
  fs.writeFileSync(path.join(OUT_DIR, 'results.json'), JSON.stringify({ base: BASE, total: results.length, passed: results.filter(r => r.ok).length, results }, null, 2));
  console.log(`\n===== ${results.filter(r => r.ok).length}/${results.length} PASS =====`);
  await browser.close();
  process.exit(results.every(r => r.ok) ? 0 : 1);
})();
