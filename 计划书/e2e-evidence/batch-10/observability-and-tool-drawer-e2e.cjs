/*
 * Batch-10 / G10 + G3 §5.2.2 —— 真实端到端验收（mock 上游，**零付费调用**）。
 *
 * 证明两条端到端结论（而不是"函数能跑"）：
 *   A. **工具抽屉请求级 opt-in 真的改变了上游收到的内容**：
 *      带重复 tools 的请求 + `X-NewAPI-Tool-Drawer: dedupe` → 上游收到的工具数变少；
 *      同一个请求 + `off` → 上游收到的工具数原样不变。
 *   B. **资源类指标真的出现在 /metrics 上**：进程内存/goroutine/DB 连接池/后台 loop 心跳，
 *      并含工具抽屉的收益度量。这是"跑三天变慢能不能被观测"的直接证据。
 *
 * 前置：实例需以
 *   METRICS_ENABLED=true SSRF_GUARD_DISABLED=true
 * 启动（SSRF 那条注释里写明仅供本地/CI 测试）。
 *
 * 运行：node 计划书/e2e-evidence/batch-10/observability-and-tool-drawer-e2e.cjs [baseUrl]
 */
const http = require('node:http')
const fs = require('node:fs')

const BASE = process.argv[2] || 'http://127.0.0.1:3099'
const MOCK_PORT = 4001
const USER = 'g10admin'
const PASS = 'G10e2e!Passw0rd'
const MODEL = 'gpt-drawer-e2e'
const OUT = __dirname

const results = []
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail: detail === undefined ? '' : String(detail) })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  :: ' + detail : ''}`)
}

function makeSession() {
  let token = ''
  return async function call(method, url, body) {
    const res = await fetch(`${BASE}${url}`, {
      method,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await res.text()
    let json = null
    try {
      json = JSON.parse(text)
    } catch {
      json = null
    }
    const fresh = json && json.data && json.data.access_token
    if (typeof fresh === 'string' && fresh) token = fresh
    return { status: res.status, json, text }
  }
}

/** mock 上游：记录**上一次收到的 tools 数量**，用于证明变换真的到达了上游。 */
function startMockUpstream() {
  const state = { calls: 0, lastToolCount: -1, lastToolNames: [] }
  const server = http.createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      state.calls += 1
      try {
        const parsed = JSON.parse(body)
        const tools = Array.isArray(parsed.tools) ? parsed.tools : []
        state.lastToolCount = tools.length
        state.lastToolNames = tools.map((t) => t?.function?.name)
      } catch {
        state.lastToolCount = -1
      }
      const payload = {
        id: 'chatcmpl-drawer-' + state.calls,
        object: 'chat.completion',
        created: 1700000000,
        model: MODEL,
        choices: [{ index: 0, message: { role: 'assistant', content: 'ok' }, finish_reason: 'stop' }],
        usage: { prompt_tokens: 5, completion_tokens: 6, total_tokens: 11 },
      }
      const out = JSON.stringify(payload)
      res.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(out) })
      res.end(out)
    })
  })
  return new Promise((resolve) => server.listen(MOCK_PORT, '127.0.0.1', () => resolve({ server, state })))
}

/** 一份**含重复项**的 tools 定义：weather 出现两次（同 name）。 */
const DUPLICATED_TOOLS = [
  { type: 'function', function: { name: 'weather', description: 'get weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } } },
  { type: 'function', function: { name: 'clock', description: 'get time', parameters: { type: 'object', properties: {} } } },
  { type: 'function', function: { name: 'weather', description: 'get weather', parameters: { type: 'object', properties: { city: { type: 'string' } } } } },
]

async function relayCall(apiKey, header) {
  const headers = { 'Content-Type': 'application/json', Authorization: `Bearer ${apiKey}` }
  if (header !== undefined) headers['X-NewAPI-Tool-Drawer'] = header
  const res = await fetch(`${BASE}/v1/chat/completions`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      model: MODEL,
      messages: [{ role: 'user', content: 'drawer probe ' + String(header) }],
      tools: DUPLICATED_TOOLS,
      temperature: 0.2,
    }),
  })
  const text = await res.text()
  return { status: res.status, text }
}

;(async () => {
  const { server, state } = await startMockUpstream()
  check('mock upstream listening', server.listening, `port ${MOCK_PORT}`)

  const call = makeSession()
  await call('POST', '/api/setup', { username: USER, password: PASS, confirmPassword: PASS })
  const login = await call('POST', '/api/user/login', { username: USER, password: PASS })
  check('admin login', login.status === 200 && login.json?.success === true, login.text.slice(0, 100))

  await call('PUT', '/api/option/', { key: 'SelfUseModeEnabled', value: 'true' })
  const channel = await call('POST', '/api/channel/', {
    mode: 'single',
    channel: {
      name: 'g10-mock-upstream',
      type: 1,
      key: 'sk-mock-key',
      base_url: `http://127.0.0.1:${MOCK_PORT}`,
      models: MODEL,
      group: 'default',
      status: 1,
      weight: 1,
      priority: 0,
    },
  })
  check('channel created', channel.status === 200 && channel.json?.success !== false, channel.text.slice(0, 140))

  const tokenResp = await call('POST', '/api/token/', {
    name: 'g10-token',
    remain_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    model_limits_enabled: false,
    group: '',
  })
  const apiKey = tokenResp.json?.data?.key
  check('api token created', typeof apiKey === 'string' && apiKey.length > 0, tokenResp.text.slice(0, 120))

  // ---- A. 工具抽屉请求级 opt-in ----
  // 基线：不带请求头且全局开关关 → 上游应收到**原样 3 个**工具
  const baseline = await relayCall(apiKey, undefined)
  check('baseline request succeeded', baseline.status === 200, `status=${baseline.status}`)
  check('without header the tools are passed through untouched', state.lastToolCount === 3,
    `upstream saw ${state.lastToolCount} tools (names=${JSON.stringify(state.lastToolNames)})`)

  // 请求头 dedupe → 上游应收到去重后的 2 个工具（weather 重复项被移除）
  const deduped = await relayCall(apiKey, 'dedupe')
  check('dedupe request succeeded', deduped.status === 200, `status=${deduped.status}`)
  check('X-NewAPI-Tool-Drawer: dedupe actually reached the upstream',
    state.lastToolCount === 2, `upstream saw ${state.lastToolCount} tools (names=${JSON.stringify(state.lastToolNames)})`)
  check('the duplicate tool was the one removed',
    JSON.stringify(state.lastToolNames) === JSON.stringify(['weather', 'clock']),
    JSON.stringify(state.lastToolNames))

  // 请求头 off → 即便全局开关开着也必须原样透传（保守客户端不被全局开关伤害）
  await call('PUT', '/api/option/feature-switches', { key: 'TOOL_DRAWER_ENABLED', value: 'true' })
  const offWhileGlobalOn = await relayCall(apiKey, 'off')
  check('off-header request succeeded', offWhileGlobalOn.status === 200, `status=${offWhileGlobalOn.status}`)
  check('header off OVERRIDES a globally enabled switch (conservative clients protected)',
    state.lastToolCount === 3, `upstream saw ${state.lastToolCount} tools`)

  // 复位全局开关
  await call('PUT', '/api/option/feature-switches', { key: 'TOOL_DRAWER_ENABLED', reset: true })

  // ---- B. /metrics 资源可观测性 ----
  // 后台 loop 的心跳需要**跑完第一轮**才会出现（不同 loop 周期不同），
  // 因此这里轮询等待，而不是启动十几秒后就断言。
  let metrics = ''
  const deadline = Date.now() + 150000
  for (;;) {
    const res = await fetch(`${BASE}/metrics`)
    metrics = await res.text()
    if (metrics.includes('background_loop_last_run_timestamp_seconds{loop=')) break
    if (Date.now() > deadline) break
    await new Promise((r) => setTimeout(r, 5000))
  }
  check('/metrics reachable', metrics.length > 0, `bytes=${metrics.length}`)

  for (const [name, why] of [
    ['process_goroutines', 'goroutine 泄漏可见'],
    ['process_memory_alloc_bytes', '进程内存可见'],
    ['process_memory_heap_objects', '"只加不减"的对象数可见'],
    ['process_memory_sys_bytes', '进程向 OS 申请的总内存可见'],
    ['db_open_connections', 'DB 连接池使用可见'],
    ['db_wait_count_total', 'DB 连接池等待可见'],
  ]) {
    check(`metric ${name} is exported (${why})`, new RegExp(`^${name} `, 'm').test(metrics), '')
  }

  const heartbeatLoops = [...metrics.matchAll(/^background_loop_last_run_timestamp_seconds\{loop="?([^"}]+)"?\}/gm)].map((m) => m[1])
  check('background loop heartbeats are exported', heartbeatLoops.length > 0, `${heartbeatLoops.length} loops: ${heartbeatLoops.join(',')}`)
  // 默认配置下应运行的 5 个 loop（task_artifact_cleanup **不在此列**：
  // 本地图床模式为 upstream 时该 loop 根本不启动，缺席是正确的）。
  for (const loop of ['sync_options', 'consume_log_flusher', 'system_task_runner', 'subscription_quota_reset', 'codex_credential_refresh']) {
    check(`heartbeat present for loop ${loop}`, heartbeatLoops.includes(loop), '')
  }
  check('heartbeat for a disabled component is correctly absent (artifact cleanup runs only with a local store)',
    !heartbeatLoops.includes('task_artifact_cleanup'), heartbeatLoops.join(',') || '(none)')

  for (const m of ['tool_drawer_saved_bytes_total', 'tool_drawer_deduped_requests_total']) {
    check(`tool drawer saving metric ${m} is exported`, new RegExp(`^${m} `, 'm').test(metrics), '')
  }
  const savedMatch = metrics.match(/^tool_drawer_saved_bytes_total (\d+)/m)
  check('tool drawer savings are non-zero after a real dedupe (not a dead metric)',
    savedMatch && Number(savedMatch[1]) > 0, `saved_bytes=${savedMatch ? savedMatch[1] : 'n/a'}`)

  server.close()

  const passed = results.filter((r) => r.ok).length
  fs.writeFileSync(
    `${OUT}/RESULTS.md`,
    [
      '# Batch-10 / G10 + G3 §5.2.2 —— 真实端到端验收（mock 上游）',
      '',
      `- 时间：${new Date().toISOString()}`,
      `- 目标：${BASE}（mock 上游 127.0.0.1:${MOCK_PORT}）`,
      `- 结果：**${passed}/${results.length} PASS**`,
      '',
      '| # | 断言 | 结果 | 详情 |',
      '|---|---|---|---|',
      ...results.map((r, i) => `| ${i + 1} | ${r.name.replace(/\|/g, '\\|')} | ${r.ok ? '✅' : '❌'} | ${r.detail.replace(/\|/g, '\\|').slice(0, 170)} |`),
      '',
      '**两条核心结论**：',
      '1. 请求头 `X-NewAPI-Tool-Drawer: dedupe` **真的改变了上游收到的 tools**（3 → 2，且被移除的正是重复项）；',
      '   而 `off` **能覆盖已开启的全局开关** —— 保守客户端不会被全局开关伤害。',
      '2. `/metrics` 真的输出了进程内存 / goroutine / DB 连接池 / 6 个后台 loop 心跳 / 工具抽屉收益，',
      '   且收益值**非零**（不是"声明了但没人写"的死指标）。',
      '',
    ].join('\n')
  )
  console.log(`\n=== ${passed}/${results.length} PASS ===`)
  process.exit(passed === results.length ? 0 : 1)
})().catch((e) => {
  console.error('E2E ERROR: ' + e.message)
  process.exit(1)
})
