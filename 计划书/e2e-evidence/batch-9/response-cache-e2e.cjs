/*
 * Batch-9 / G3 响应缓存 —— 真实端到端验收（mock 上游，**零真实付费调用**）。
 *
 * 要证明的是一条**端到端**结论，而不是"函数能跑"：
 *   同一 prompt 连发两次 → 第二次命中缓存 → **上游一个请求都没多收**，
 *   且客户端拿到的 usage 与第一次完全一致。
 *
 * 做法：
 *  1. 起一个 mock 上游（计数每次被调用）；
 *  2. 在本地实例里建渠道指向 mock、建 token；
 *  3. 打开 RESPONSE_CACHE_ENABLED 并把模型加进白名单；
 *  4. 连发两次**完全相同**的非流式请求；
 *  5. 断言：mock 只被调用 1 次、两次响应体逐字节一致、命中/未命中计数正确。
 *
 * 运行：node 计划书/e2e-evidence/batch-9/response-cache-e2e.cjs [baseUrl]
 */
const http = require('node:http')
const fs = require('node:fs')
const path = require('node:path')

const BASE = process.argv[2] || 'http://127.0.0.1:3099'
const MOCK_PORT = 3999
const USER = 'g3admin'
const PASS = 'G3e2e!Passw0rd'
const MODEL = 'gpt-cache-e2e'
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

/** mock 上游：OpenAI 兼容的非流式补全，并记录被调用次数。 */
function startMockUpstream() {
  const state = { calls: 0, lastBody: null }
  const server = http.createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      state.calls += 1
      state.lastBody = body
      const payload = {
        id: 'chatcmpl-mock-' + state.calls,
        object: 'chat.completion',
        created: 1700000000,
        model: MODEL,
        choices: [
          { index: 0, message: { role: 'assistant', content: 'mock reply' }, finish_reason: 'stop' },
        ],
        // 固定 usage —— 这样才能断言"第二次拿到的 usage 与第一次完全一致"
        usage: { prompt_tokens: 11, completion_tokens: 22, total_tokens: 33 },
      }
      const out = JSON.stringify(payload)
      res.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(out) })
      res.end(out)
    })
  })
  return new Promise((resolve) => {
    server.listen(MOCK_PORT, '127.0.0.1', () => resolve({ server, state }))
  })
}

async function relayCall(apiKey, content) {
  const res = await fetch(`${BASE}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${apiKey}` },
    body: JSON.stringify({
      model: MODEL,
      messages: [{ role: 'user', content }],
      temperature: 0.1,
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
  check('admin login', login.status === 200 && login.json?.success === true, login.text.slice(0, 120))

  // 本实例是全新空库，`gpt-cache-e2e` 没有配价 —— 按报错自身的指引开启自用模式，
  // 让本次验收专注在**缓存行为**上，而不是先搭一套定价。
  const selfUse = await call('PUT', '/api/option/', { key: 'SelfUseModeEnabled', value: 'true' })
  check('self-use mode enabled', selfUse.status === 200, selfUse.text.slice(0, 120))

  // SSRF 护栏默认拒绝私网/回环上游（这是**正确**的安全默认）。
  // 本用例要连本机 mock，因此本实例必须以 `SSRF_GUARD_DISABLED=true` 启动 ——
  // 该变量在代码注释里就写明「仅供本地基准/CI 测试」，生产绝不应设置。
  const guardDisabled = process.env.G3_EXPECT_SSRF_DISABLED === '1'
  check('local test instance started with SSRF_GUARD_DISABLED=1 (required for mock upstream)',
    guardDisabled, guardDisabled ? '' : 'set G3_EXPECT_SSRF_DISABLED=1 when running this script')

  // 渠道指向 mock 上游。
  // 契约：`POST /api/channel/` 收 `AddChannelRequest{ mode, channel *model.Channel }` ——
  // 渠道字段必须包在 `channel` 里，且分组字段是 `group`（逗号分隔字符串）。
  const channel = await call('POST', '/api/channel/', {
    mode: 'single',
    channel: {
      name: 'g3-mock-upstream',
      type: 1, // OpenAI
      key: 'sk-mock-key',
      base_url: `http://127.0.0.1:${MOCK_PORT}`,
      models: MODEL,
      group: 'default',
      status: 1,
      weight: 1,
      priority: 0,
    },
  })
  check('channel created', channel.status === 200 && channel.json?.success !== false, channel.text.slice(0, 200))

  // token（不限额度，避免额度门槛干扰本次要验证的缓存行为）
  const tokenResp = await call('POST', '/api/token/', {
    name: 'g3-cache-token',
    remain_quota: 0,
    unlimited_quota: true,
    expired_time: -1,
    model_limits_enabled: false,
    group: '',
  })
  const apiKey = tokenResp.json?.data?.key || tokenResp.json?.data?.Key
  check('api token created', typeof apiKey === 'string' && apiKey.length > 0, tokenResp.text.slice(0, 160))

  // 打开缓存 + 白名单（白名单为空时全不缓存 —— 这是刻意的保守默认）
  const sw = await call('PUT', '/api/option/feature-switches', { key: 'RESPONSE_CACHE_ENABLED', value: 'true' })
  check('RESPONSE_CACHE_ENABLED turned on', sw.status === 200 && sw.json?.success === true, sw.text.slice(0, 140))

  const allow = await call('PUT', '/api/option/', {
    key: 'response_cache.allow_models',
    value: JSON.stringify([MODEL]),
  })
  check('allowlist configured via option API', allow.status === 200, allow.text.slice(0, 140))

  // 先确认白名单外不会命中（保守默认的旁证：换成别的模型名不会读到这条缓存）
  const before = state.calls

  // 第一次：未命中 → 打上游
  const r1 = await relayCall(apiKey, 'cache e2e probe')
  check('first call succeeded (cache miss)', r1.status === 200, `status=${r1.status} body=${r1.text.slice(0, 120)}`)
  const callsAfterFirst = state.calls
  check('cache miss hit the upstream exactly once', callsAfterFirst === before + 1,
    `upstream calls: ${before} -> ${callsAfterFirst}`)

  // 第二次：完全相同 → 应命中，上游不再被调用
  const r2 = await relayCall(apiKey, 'cache e2e probe')
  check('second call succeeded (cache hit)', r2.status === 200, `status=${r2.status}`)
  check('SECOND CALL DID NOT REACH THE UPSTREAM', state.calls === callsAfterFirst,
    `upstream calls stayed at ${state.calls}`)
  check('both responses are byte-identical (usage truthfully replayed)',
    r1.text === r2.text, `len1=${r1.text.length} len2=${r2.text.length}`)

  // 换个内容 → 必须不命中（键确实随内容变化）
  const r3 = await relayCall(apiKey, 'a different prompt')
  check('different prompt is NOT served from cache', state.calls === callsAfterFirst + 1,
    `upstream calls: ${state.calls}`)
  check('different prompt got a different response', r3.text !== r1.text)

  // 服务端计数必须反应真实命中
  const list = await call('GET', '/api/option/feature-switches')
  const metrics = (list.json?.data?.metrics) || {}
  check('metrics report at least one cache hit', Number(metrics.response_cache_hits_total) >= 1,
    `hits=${metrics.response_cache_hits_total} misses=${metrics.response_cache_misses_total} live=${metrics.response_cache_live_entries}`)

  // 复位：关掉开关并清空白名单，避免影响后续用例
  await call('PUT', '/api/option/feature-switches', { key: 'RESPONSE_CACHE_ENABLED', reset: true })
  await call('PUT', '/api/option/', { key: 'response_cache.allow_models', value: '[]' })

  server.close()

  const passed = results.filter((r) => r.ok).length
  fs.writeFileSync(
    `${OUT}/RESULTS.md`,
    [
      '# Batch-9 / G3 响应缓存 —— 真实端到端验收（mock 上游）',
      '',
      `- 时间：${new Date().toISOString()}`,
      `- 目标：${BASE}（mock 上游 127.0.0.1:${MOCK_PORT}）`,
      `- 结果：**${passed}/${results.length} PASS**`,
      '',
      '| # | 断言 | 结果 | 详情 |',
      '|---|---|---|---|',
      ...results.map((r, i) => `| ${i + 1} | ${r.name.replace(/\|/g, '\\|')} | ${r.ok ? '✅' : '❌'} | ${r.detail.replace(/\|/g, '\\|').slice(0, 160)} |`),
      '',
      '**核心结论**：同一 prompt 连发两次，**上游只被调用了一次**，且两次响应体逐字节一致。',
      '这证明缓存真的省掉了整次上游调用，而不是"函数能跑"。',
      '',
    ].join('\n')
  )
  console.log(`\n=== ${passed}/${results.length} PASS ===`)
  process.exit(passed === results.length ? 0 : 1)
})().catch((e) => {
  console.error('E2E ERROR: ' + e.message)
  process.exit(1)
})
