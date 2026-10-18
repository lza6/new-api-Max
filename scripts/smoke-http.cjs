// 真实服务端 HTTP E2E（本地，SQLite，不联网、不调用付费 API）。
//
// 目的：把此前「handler 级」的验证升级为**真实路由 + 真实鉴权 + 真实序列化**。
// handler 级测试绕过了路由注册与中间件，历史上有过「库全绿但线上 404」的事故。
//
// 用法：
//   1) 先起一个**本地独立实例**（不要指向生产）：
//      go build -o /tmp/new-api-smoke.exe .
//      SQLITE_PATH=/tmp/na-smoke/smoke.db PORT=18123 SESSION_SECRET=local-only GIN_MODE=release /tmp/new-api-smoke.exe
//   2) node scripts/smoke-http.cjs http://127.0.0.1:18123
//
// 默认**不配置 TRAINING_DATA_SINK**（云盘后端 disabled），因此本脚本验证的是
// 「未配置云盘时各端点仍能正确响应」，而不是真实上传。这正好覆盖了
// 「管理员没配云盘」这个最常见的部署状态。
//
// 不联网、不调用任何付费 API、不写入生产。
const BASE = process.argv[2] || 'http://127.0.0.1:18123'

let cookie = ''
let accessToken = '' // 登录返回的 access_token，管理端请求要走 Authorization 头
let pass = 0
let fail = 0
const failures = []

async function req(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json', ...(opts.headers || {}) }
  if (cookie) headers['Cookie'] = cookie
  if (accessToken) headers['Authorization'] = 'Bearer ' + accessToken
  const res = await fetch(BASE + path, { ...opts, headers, redirect: 'manual' })
  const setCookie = res.headers.getSetCookie?.() || []
  if (setCookie.length) {
    cookie = setCookie.map((c) => c.split(';')[0]).join('; ')
  }
  const text = await res.text()
  let json = null
  try {
    json = JSON.parse(text)
  } catch {
    /* 非 JSON（如 SPA HTML） */
  }
  return { status: res.status, text, json }
}

function check(name, cond, detail) {
  if (cond) {
    pass++
    console.log(`  ✅ ${name}`)
  } else {
    fail++
    failures.push(name)
    console.log(`  ❌ ${name}${detail ? ' — ' + detail : ''}`)
  }
}

async function waitReady(timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const r = await req('/api/status')
      if (r.status === 200) return true
    } catch {
      /* 还没起来 */
    }
    await new Promise((r) => setTimeout(r, 500))
  }
  return false
}

;(async () => {
  console.log(`\n=== HTTP E2E @ ${BASE} ===\n`)

  if (!(await waitReady())) {
    console.log('❌ 服务未在超时内就绪')
    process.exit(1)
  }
  console.log('服务已就绪\n')

  // ---------- 1. 首次初始化 ----------
  console.log('[1] 初始化 root 账号')
  const setup = await req('/api/setup', {
    method: 'POST',
    body: JSON.stringify({
      username: 'smoke',
      password: 'SmokeTest123456',
      confirmPassword: 'SmokeTest123456',
      SelfUseModeEnabled: false,
      DemoSiteEnabled: false,
    }),
  })
  // 已初始化过（重复运行）也算通过
  check(
    'setup 接口可达且返回结构正确',
    setup.json && typeof setup.json.success === 'boolean',
    `status=${setup.status} body=${setup.text.slice(0, 120)}`
  )

  // ---------- 2. 登录 ----------
  console.log('\n[2] 登录')
  const login = await req('/api/user/login', {
    method: 'POST',
    body: JSON.stringify({ username: 'smoke', password: 'SmokeTest123456' }),
  })
  check('登录成功', login.json && login.json.success === true, `body=${login.text.slice(0, 200)}`)
  accessToken = login.json?.data?.access_token || ''
  check('拿到 access_token', accessToken !== '', `keys=${Object.keys(login.json?.data || {}).join(',')}`)

  if (!login.json || !login.json.success) {
    console.log('\n⚠️ 登录失败，后续管理端用例无法执行（可能密码非首次初始化设置）')
  }

  // ---------- 3. 用户侧：采集授权状态 ----------
  console.log('\n[3] 用户侧接口')
  const consent = await req('/api/user/training-data/status')
  check(
    'GET /api/user/training-data/status 返回信封',
    consent.json && consent.json.success === true,
    `status=${consent.status} body=${consent.text.slice(0, 160)}`
  )
  // 未配置云盘 → available 必须为 false（不能给一个点了没用的开关）
  check(
    '未配置云盘时 available=false（不渲染无效开关）',
    consent.json?.data?.available === false,
    `data=${JSON.stringify(consent.json?.data)}`
  )

  // ---------- 4. 管理端：训练数据 ----------
  console.log('\n[4] 管理端训练数据接口')
  const overview = await req('/api/admin/training-data/overview')
  check(
    'GET overview 200 且 success',
    overview.status === 200 && overview.json?.success === true,
    `status=${overview.status} body=${overview.text.slice(0, 160)}`
  )

  // ★ B1 回归：默认路径（不传 date）必须有响应体与 days 字段
  const byDate = await req('/api/admin/training-data/by-date')
  check(
    'GET by-date（不传 date）返回非空 JSON 且 success',
    byDate.status === 200 && byDate.json?.success === true,
    `status=${byDate.status} body=${byDate.text.slice(0, 200)}`
  )
  check(
    'by-date 下发 days 字段（每日历史卡依赖它）',
    Array.isArray(byDate.json?.data?.days),
    `data=${JSON.stringify(byDate.json?.data)?.slice(0, 200)}`
  )

  const today = new Date().toISOString().slice(0, 10)
  const byDateToday = await req(`/api/admin/training-data/by-date?date=${today}`)
  check(
    'GET by-date?date=<今天> 返回 success（曾因漏写响应体而空 body）',
    byDateToday.status === 200 && byDateToday.json?.success === true,
    `body=${byDateToday.text.slice(0, 200)}`
  )

  const badDate = await req('/api/admin/training-data/by-date?date=2026-13-99')
  check('非法日期被拒绝', badDate.json?.success === false, `body=${badDate.text.slice(0, 120)}`)

  // ★ 路径穿越回归
  const traversal = await req('/api/admin/training-data/files?path=..%2Fsecret')
  check(
    '列举接口拒绝上跳路径',
    traversal.json?.success === false && /invalid path/.test(traversal.json?.message || ''),
    `body=${traversal.text.slice(0, 160)}`
  )

  // ---------- 5. 管理端：运营面板 ----------
  console.log('\n[5] 运营面板接口')
  const opsOverview = await req('/api/admin/ops/overview')
  check(
    'GET /api/admin/ops/overview 成功',
    opsOverview.status === 200 && opsOverview.json?.success === true,
    `status=${opsOverview.status} body=${opsOverview.text.slice(0, 160)}`
  )
  const rank = await req('/api/admin/ops/consumption-ranking?days=7&limit=20')
  check(
    'GET consumption-ranking 成功且 items 为数组',
    rank.status === 200 && rank.json?.success === true && Array.isArray(rank.json?.data?.items),
    `body=${rank.text.slice(0, 200)}`
  )

  // ---------- 6. 管理端：Web 防护 ----------
  console.log('\n[6] Web 防护接口')
  const wps = await req('/api/admin/web-protection/settings')
  check(
    'GET web-protection/settings 成功',
    wps.status === 200 && wps.json?.success !== false,
    `status=${wps.status} body=${wps.text.slice(0, 160)}`
  )

  // ---------- 7. 鉴权边界 ----------
  console.log('\n[7] 鉴权边界（无会话不得访问管理端）')
  const savedToken = accessToken
  accessToken = ''
  const noAuth = await req('/api/admin/training-data/overview')
  check(
    '无会话访问管理端被拒绝',
    noAuth.status === 401 || noAuth.status === 403 || noAuth.json?.success === false,
    `status=${noAuth.status} body=${noAuth.text.slice(0, 120)}`
  )
  accessToken = savedToken

  // ---------- 汇总 ----------
  console.log(`\n=== 结果：${pass} 通过 / ${fail} 失败 ===`)
  if (fail) {
    console.log('失败项：')
    failures.forEach((f) => console.log('  - ' + f))
    process.exit(1)
  }
  process.exit(0)
})().catch((e) => {
  console.error('脚本异常：', e)
  process.exit(1)
})
