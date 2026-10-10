/*
 * Batch-8 / G1 —— 「跨进程重启」持久化验证（**接口层真凭据**）。
 *
 * 为什么单独做：浏览器刷新只证明内存态还在；只有**重启进程**后配置仍生效，
 * 才证明真的落到了 option 表。本脚本自己拉起/杀掉服务进程。
 *
 * 前置：目标实例的 SQLite 库位于 <workdir>/one-api.db，二进制可执行。
 * 运行：node 计划书/e2e-evidence/batch-8/persistence-restart-e2e.cjs <bin> <workdir> <port>
 */
const { spawn } = require('node:child_process')
const path = require('node:path')
const fs = require('node:fs')

const BIN = process.argv[2]
const WORKDIR = process.argv[3] || '/tmp/b8e2e'
const PORT = process.argv[4] || '3099'
const BASE = `http://127.0.0.1:${PORT}`
const USER = 'e2eadmin'
const PASS = 'B8e2e!Passw0rd'
const KEY = 'RELAY_AUDIT_ENABLED'

const results = []
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail: detail === undefined ? '' : String(detail) })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  :: ' + detail : ''}`)
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms))
}

async function waitHealthy(timeoutMs = 60000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const r = await fetch(`${BASE}/healthz`)
      if (r.status === 200) return true
    } catch {
      /* not up yet */
    }
    await sleep(1000)
  }
  return false
}

function startServer() {
  const log = fs.openSync(path.join(WORKDIR, 'server.log'), 'a')
  const child = spawn(BIN, [], {
    cwd: WORKDIR,
    env: { ...process.env, PORT: String(PORT) },
    stdio: ['ignore', log, log],
    detached: false,
  })
  return child
}

async function stopServer(child) {
  if (!child) return
  child.kill('SIGKILL')
  await sleep(2500)
}

/** 带会话调接口：优先用登录返回的 access_token（Bearer），Cookie 作为兜底。 */
function makeSession() {
  let cookie = ''
  let token = ''
  return async function call(method, url, body) {
    const res = await fetch(`${BASE}${url}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(cookie ? { Cookie: cookie } : {}),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const setCookie = res.headers.getSetCookie ? res.headers.getSetCookie() : []
    if (setCookie.length) {
      cookie = setCookie.map((c) => c.split(';')[0]).join('; ')
    }
    const text = await res.text()
    let json = null
    try {
      json = JSON.parse(text)
    } catch {
      json = null
    }
    // 登录/刷新响应带 access_token —— 后续管理接口用它做 Bearer 鉴权。
    const fresh = json && json.data && json.data.access_token
    if (typeof fresh === 'string' && fresh) token = fresh
    return { status: res.status, json, text }
  }
}

;(async () => {
  // ---- 阶段 1：干净启动 ----
  await fetch(`${BASE}/healthz`).catch(() => {})
  let server = startServer()
  check('server started', await waitHealthy(), BASE)

  const call = makeSession()
  const setup = await call('POST', '/api/setup', {
    username: USER,
    password: PASS,
    confirmPassword: PASS,
  })
  check('setup/login available',
    setup.status === 200 && (setup.json?.success === true || String(setup.json?.message || '').includes('已经初始化完成')),
    setup.text.slice(0, 100))

  const login = await call('POST', '/api/user/login', { username: USER, password: PASS })
  check('login', login.status === 200 && login.json?.success === true, login.text.slice(0, 100))

  // ---- 阶段 2：打开开关 ----
  const on = await call('PUT', '/api/option/feature-switches', { key: KEY, value: 'true' })
  check('enable switch via API', on.status === 200 && on.json?.success === true, on.text.slice(0, 160))

  const before = await call('GET', '/api/option/feature-switches')
  const s1 = (before.json?.data?.switches || []).find((s) => s.key === KEY)
  check('switch on before restart', s1 && s1.value === 'true' && s1.configured === true, JSON.stringify(s1))

  // ---- 阶段 3：重启进程 ----
  await stopServer(server)
  server = startServer()
  const up = await waitHealthy()
  check('server restarted', up, BASE)

  const call2 = makeSession()
  await call2('POST', '/api/user/login', { username: USER, password: PASS })

  // ---- 阶段 4：重启后配置仍在（这是唯一能证明"落库"的证据）----
  const after = await call2('GET', '/api/option/feature-switches')
  const s2 = (after.json?.data?.switches || []).find((s) => s.key === KEY)
  check('switch STILL ON after process restart (persisted to DB)',
    s2 && s2.value === 'true' && s2.configured === true && s2.effective === true,
    JSON.stringify(s2))

  // ---- 阶段 5：还原（reset 也必须在重启后保持）----
  const reset = await call2('PUT', '/api/option/feature-switches', { key: KEY, reset: true })
  check('reset switch via API', reset.status === 200 && reset.json?.success === true, reset.text.slice(0, 160))

  await stopServer(server)
  server = startServer()
  check('server restarted (2nd)', await waitHealthy(), BASE)
  const call3 = makeSession()
  await call3('POST', '/api/user/login', { username: USER, password: PASS })
  const after2 = await call3('GET', '/api/option/feature-switches')
  const s3 = (after2.json?.data?.switches || []).find((s) => s.key === KEY)
  check('reset ALSO persisted across restart (back to env default)',
    s3 && s3.configured === false && s3.value === 'false',
    JSON.stringify(s3))

  await stopServer(server)

  const passed = results.filter((r) => r.ok).length

  fs.writeFileSync(
    `${__dirname}/RESULTS-persistence-restart.md`,
    [
      '# Batch-8 / G1 跨进程重启持久化验证',
      '',
      `- 时间：${new Date().toISOString()}`,
      `- 目标：${BASE}（二进制 ${BIN}，工作目录 ${WORKDIR}）`,
      `- 结果：**${passed}/${results.length} PASS**`,
      '',
      '| # | 断言 | 结果 | 详情 |',
      '|---|---|---|---|',
      ...results.map((r, i) => `| ${i + 1} | ${r.name} | ${r.ok ? '✅' : '❌'} | ${r.detail.replace(/\|/g, '\\|').slice(0, 180)} |`),
      '',
    ].join('\n')
  )
  console.log(`\n=== ${passed}/${results.length} PASS ===`)
  process.exit(passed === results.length ? 0 : 1)
})().catch(async (e) => {
  console.error('ERROR: ' + e.message)
  process.exit(1)
})
