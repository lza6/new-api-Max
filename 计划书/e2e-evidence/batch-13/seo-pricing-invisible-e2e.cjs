// v1.3.132 生产验收：/pricing 的服务端 SEO 注入块必须对用户不可见，
// 且不得在页面上留下爬虫专用的文案。
//
// 复现的缺陷（用户截图确认）：价目表被拼在 `#spa-seo-content` 之外，
// 清理脚本删不掉它 → 表格永久显示在页面顶部。
//
// 用法（在仓库根运行）：node 计划书/e2e-evidence/batch-13/seo-pricing-invisible-e2e.cjs
const path = require('node:path')
const https = require('node:https')
const { chromium, devices } = require(
  path.resolve('web/node_modules/playwright')
)

const BASE = 'https://freeapi.tingfengai.art'
const DOMAIN = 'freeapi.tingfengai.art'
let pass = 0
let fail = 0
const check = (name, ok, detail) => {
  if (ok) {
    pass++
    console.log(`  PASS  ${name}`)
  } else {
    fail++
    console.log(`  FAIL  ${name}${detail ? ' :: ' + detail : ''}`)
  }
}

/**
 * 经本地隧道读取生产原始 HTML。
 *
 * 本机直连该域名的 TLS 会被中间设备注入 RST（见记忆 `local-curl-schannel-broken`），
 * 因此必须走 127.0.0.1:443 的 SSH 隧道 + 显式 servername。
 */
function rawHtml(urlPath) {
  return new Promise((resolve) => {
    https
      .get(
        {
          host: '127.0.0.1',
          port: 443,
          path: urlPath,
          servername: DOMAIN,
          agent: new https.Agent({ rejectUnauthorized: false }),
          headers: { Host: DOMAIN, 'Accept-Language': 'zh-CN' },
        },
        (r) => {
          const c = []
          r.on('data', (x) => c.push(x))
          r.on('end', () => resolve(Buffer.concat(c).toString()))
        }
      )
      .on('error', () => resolve(''))
  })
}

const state = (p) =>
  p.evaluate(() => {
    const text = document.body.innerText || ''
    return {
      seoBlock: !!document.getElementById('spa-seo-content'),
      priceTable: !!document.getElementById('spa-seo-pricing'),
      path: location.pathname,
      rootChildren: document.getElementById('root')?.childElementCount || 0,
      leakedPricingText: text.includes('见本页交互界面'),
      leakedBillingMode: text.includes('按用量表达式计费'),
    }
  })

;(async () => {
  const b = await chromium.launch({
    args: [
      '--no-proxy-server',
      '--proxy-bypass-list=*',
      `--host-resolver-rules=MAP ${DOMAIN} 127.0.0.1`,
    ],
    proxy: { server: 'direct://' },
  })
  const ctx = await b.newContext({ ...devices['Desktop Chrome'] })
  const p = await ctx.newPage()
  const errors = []
  p.on('pageerror', (e) => errors.push(String(e)))

  console.log(`\n=== v1.3.132 SEO 注入可见性验收 @ ${BASE} ===\n`)

  console.log('[1] 直接访问 /pricing（全新加载）')
  await p.goto(`${BASE}/pricing`, { waitUntil: 'domcontentloaded', timeout: 90000 })

  // 挂载**前**取样：如实记录 SEO 注入块的可见窗口。
  // 这一步不是为了断言"看不见"（它在这个时点**本来就看得见**），而是把事实记录下来，
  // 避免再用"挂载后取样"制造出"用户全程看不到"的假象。
  const preMount = await p.evaluate(() => {
    const el = document.getElementById('spa-seo-content')
    const tbl = document.getElementById('spa-seo-pricing')
    const style = el ? getComputedStyle(el) : null
    return {
      seoBlockInDom: !!el,
      priceTableInDom: !!tbl,
      display: style ? style.display : null,
      visibility: style ? style.visibility : null,
      // 该元素在当前时点是否实际占据可见区域
      hasVisibleBox: el ? el.getBoundingClientRect().height > 0 : false,
    }
  })
  console.log('    挂载前：' + JSON.stringify(preMount))
  console.log(
    '    （说明：注入块在 React 挂载前**对用户可见** —— 它是服务端降级内容，' +
      '刻意不用 CSS 隐藏，因为 Googlebot 会应用 CSS，隐藏只让爬虫读会构成 cloaking）'
  )
  check(
    '挂载前的注入块状态已被记录（display/visibility 未被 CSS 隐藏）',
    preMount.display !== 'none' || preMount.display === null,
    JSON.stringify(preMount)
  )

  await p
    .waitForFunction(() => (document.getElementById('root')?.childElementCount || 0) > 0, {
      timeout: 60000,
    })
    .catch(() => {})
  await p.waitForTimeout(1500)
  let s = await state(p)
  console.log('    挂载后：' + JSON.stringify(s))
  check('SEO 内容块已从 DOM 移除', !s.seoBlock)
  check('价目表已从 DOM 移除（本次修复的核心）', !s.priceTable, '表格仍在 DOM 中')
  check('页面上没有残留的爬虫价目表文案', !s.leakedPricingText)
  check('React 已渲染（#root 有子节点）', s.rootChildren > 0)

  console.log('\n[2] 客户端路由切换后不得残留')
  // 用页面内 history 推送模拟 SPA 导航（不重载文档），这是残留会"跟着走"的场景
  await p.evaluate(() => {
    history.pushState({}, '', '/rankings')
    window.dispatchEvent(new PopStateEvent('popstate'))
  })
  await p.waitForTimeout(2500)
  s = await state(p)
  console.log('    ' + JSON.stringify(s))
  check('客户端切换后 SEO 内容块不存在', !s.seoBlock)
  check('客户端切换后价目表不存在', !s.priceTable)

  console.log('\n[3] 其它公开页同样不可见')
  for (const route of ['/', '/rankings', '/about']) {
    await p.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 90000 })
    await p
      .waitForFunction(() => (document.getElementById('root')?.childElementCount || 0) > 0, {
        timeout: 60000,
      })
      .catch(() => {})
    await p.waitForTimeout(1200)
    const st = await state(p)
    check(`${route} 无 SEO 块残留`, !st.seoBlock && !st.priceTable, JSON.stringify(st))
  }

  console.log('\n[4] 服务端 HTML 侧（爬虫视角，应保留内容）')
  for (const route of ['/pricing', '/']) {
    const html = await rawHtml(route)
    check(`${route} 服务端 HTML 仍有 #spa-seo-content（爬虫能看到）`, html.includes('spa-seo-content'))
    check(`${route} 服务端 HTML 含移除脚本`, html.includes('MutationObserver'))
  }
  const pricingHtml = await rawHtml('/pricing')
  const tbl = pricingHtml.match(/<table id="spa-seo-pricing">([\s\S]*?)<\/table>/)
  if (tbl) {
    const rows = [
      ...tbl[1].matchAll(/<tr><td>([^<]*)<\/td><td>([^<]*)<\/td><td>([^<]*)<\/td><\/tr>/g),
    ].map((m) => ({ name: m[1], mode: m[2], price: m[3] }))
    console.log('    服务端价目表（爬虫视角）：')
    rows.forEach((r) => console.log(`      ${r.name} | ${r.mode} | ${r.price}`))

    // 与前端一致的规则（web/src/features/pricing/lib/billing-mode.ts 的
    // getBillingModeLabelKey，**优先级从高到低**）：
    //   hasTaskUsageSchema → 'Task billing'
    //   isDynamicPricingModel(billing_mode==='tiered_expr' && billing_expr) → 'Dynamic Pricing'
    //   isTokenBasedModel(quota_type!==1) → 'Token-based'
    //   else → 'Per Request'
    // 只有「按次计费」才展示固定单价；其余三类都不展示（Go 侧无等价实现，报数即误导）。
    const apiModels = JSON.parse((await rawHtml('/api/pricing')) || '{}').data || []
    const byName = new Map(apiModels.map((m) => [m.model_name, m]))
    const mismatched = []
    for (const r of rows) {
      const m = byName.get(r.name)
      if (!m) continue
      const hasTaskSchema =
        m.billing_usage_schema != null && Object.keys(m.billing_usage_schema).length > 0
      const isDynamic = m.billing_mode === 'tiered_expr' && Boolean(m.billing_expr)
      let wantMode
      if (hasTaskSchema) wantMode = '任务计费'
      else if (isDynamic) wantMode = '按用量表达式计费'
      else if (m.quota_type !== 1) wantMode = '按 token 计费'
      else wantMode = '按次计费'

      if (r.mode !== wantMode) mismatched.push(`${r.name}: 期望 ${wantMode} 实得 ${r.mode}`)
      const showsFixedPrice = r.price.startsWith('$')
      if (wantMode === '按次计费' && !showsFixedPrice) {
        mismatched.push(`${r.name}: 按次计费却没有固定价 ${r.price}`)
      }
      if (wantMode !== '按次计费' && showsFixedPrice) {
        mismatched.push(`${r.name}: ${wantMode} 却展示了固定价 ${r.price}`)
      }
    }
    check(
      '计费方式与固定价展示与前端判据一致（任务 > 表达式 > token > 按次）',
      mismatched.length === 0,
      mismatched.join(' | ')
    )
  } else {
    console.log('    （本次响应无价目表 —— 未登录可见模型为 0）')
  }

  check('无页面 JS 错误', errors.length === 0, errors.join(' | '))

  await p.goto(`${BASE}/pricing`, { waitUntil: 'networkidle', timeout: 90000 }).catch(() => {})
  await p.waitForTimeout(1500)
  await p.screenshot({ path: '/tmp/v13132-pricing-after.png', fullPage: false })

  console.log(`\n=== 结果：${pass} PASS / ${fail} FAIL ===\n`)
  await b.close()
  process.exit(fail === 0 ? 0 : 1)
})().catch((e) => {
  console.error(e)
  process.exit(1)
})
