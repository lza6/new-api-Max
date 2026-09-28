// new-api v1.3.56 断点截图 E2E（playwright-core + 系统 Chrome）
// 目标：webhook 设置页 / keys step-up / 定价订阅统计卡 在 375/768/1280 三断点截图 + 无横向溢出断言
// 依赖：本地已起服务（BASE 默认 127.0.0.1:18601，可改环境变量 BASE）
// 用法：node browser-e2e-v1.3.56-a11y.cjs
const { chromium } = require('C:/Users/Administrator.DESKTOP-EGNE9ND/AppData/Local/Temp/browser-e2e/node_modules/playwright-core');
const fs = require('fs');
const path = require('path');

const BASE = process.env.BASE || 'http://127.0.0.1:3000';
const CHROME = 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const OUT_DIR = path.resolve('计划书/e2e-evidence/browser-e2e-v1.3.56-a11y');
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
  await page.screenshot({ path: path.join(OUT_DIR, name), fullPage: false });
}

// 无横向溢出检测：document.scrollWidth 不应超过 viewport width（允许 1px 误差）
async function assertNoHOverflow(page, label, viewportW) {
  const sw = await page.evaluate(() => document.documentElement.scrollWidth);
  const bodySw = await page.evaluate(() => document.body.scrollWidth);
  const ok = sw <= viewportW + 1 && bodySw <= viewportW + 1;
  record(`${label} 无横向溢出`, ok, `docSW=${sw} bodySW=${bodySw} vw=${viewportW}`);
  return ok;
}

(async () => {
  fs.mkdirSync(OUT_DIR, { recursive: true });
  const browser = await chromium.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox'] });

  for (const vp of VIEWPORTS) {
    const context = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, locale: 'en-US' });
    const page = await context.newPage();
    page.setDefaultTimeout(45000);

    // 1) 定价页 + 订阅统计卡（公开）
    await page.goto(`${BASE}/pricing`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(4000);
    const statsCount = await page.locator('text=Active subscriptions').count();
    record(`[${vp.name}] pricing 渲染统计卡`, statsCount > 0, `statsCard=${statsCount}`);
    await assertNoHOverflow(page, `[${vp.name}] pricing`, vp.width);
    await shot(page, `${vp.name}-pricing.png`);

    // 2) 登录 root（若需要）
    await page.goto(`${BASE}/sign-in`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2000);
    const userInput = await page.locator('input[name="username"]').count();
    if (userInput > 0) {
      await page.fill('input[name="username"]', 'root');
      await page.fill('input[name="password"]', 'Admin-2026!');
      await page.locator('button[type="submit"]').first().click();
      await page.waitForTimeout(4000);
    }

    // 3) webhook 设置页（admin）
    await page.goto(`${BASE}/webhook`, { waitUntil: 'domcontentloaded' }).catch(() => {
      // 路径不对时退回登录后可访问的路径
    });
    await page.waitForTimeout(3500);
    const webhookTitle = await page.locator('text=Webhook Notifications').count();
    record(`[${vp.name}] webhook 设置页渲染`, webhookTitle > 0, `count=${webhookTitle}`);
    await assertNoHOverflow(page, `[${vp.name}] webhook`, vp.width);
    if (webhookTitle > 0) {
      await shot(page, `${vp.name}-webhook.png`);
    }

    // 4) keys 页 + step-up 弹窗
    await page.goto(`${BASE}/keys`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(4000);
    const masked = await page.locator('span:text-matches("^sk-")').first().count();
    if (masked > 0) {
      await page.locator('span:text-matches("^sk-")').first().click();
      await page.waitForTimeout(2500);
      const dlg = await page.locator('text=Verify to view API key').count();
      record(`[${vp.name}] keys step-up 弹窗`, dlg > 0, `dlg=${dlg}`);
      if (dlg > 0) {
        await assertNoHOverflow(page, `[${vp.name}] keys step-up`, vp.width);
        await shot(page, `${vp.name}-keys-stepup.png`);
      }
    } else {
      record(`[${vp.name}] keys step-up 弹窗`, false, '未找到 masked sk- token（可能无 token 或无权限）');
    }

    await context.close();
  }

  await browser.close();
  const passed = results.filter((r) => r.ok).length;
  const failed = results.filter((r) => !r.ok).length;
  console.log(`\n==== 汇总: ${passed} PASS / ${failed} FAIL / ${results.length} total ====`);
  fs.writeFileSync(path.join(OUT_DIR, 'summary.json'), JSON.stringify(results, null, 2));
  process.exit(failed > 0 ? 1 : 0);
})().catch((e) => {
  console.error('E2E ERROR:', e.message);
  process.exit(1);
});
