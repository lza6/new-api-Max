// Batch-13 压测：新增统计端点与 SEO 注入页在并发下的稳定性。
//
// 目的（对应"极端以及高并发"要求）：
//   1. 新增的公开端点 `/api/site/visitors` 有没有 5xx / 超时 / 明显退化；
//   2. 服务端 SEO 注入（含价目表渲染）在并发下是否稳定 —— 它每次请求都要
//      重新拼 HTML + 查一次定价，属于本次新增的热路径；
//   3. 写入侧（访客统计）在并发下会不会丢数据或报错。
//
// 约束：**打的是生产**，因此并发与总请求数都取保守值，并在结束后核对
// 容器健康与 5xx 计数。所有请求经本地 SSH 隧道（本机直连该域名会被 RST）。
//
// 用法：node stress-site-visitors.cjs [并发] [每并发请求数]
const http = require('node:http')
const https = require('node:https')

const CONCURRENCY = Number(process.argv[2] || 12)
const PER_WORKER = Number(process.argv[3] || 20)
const DOMAIN = 'freeapi.tingfengai.art'
const agent = new https.Agent({ rejectUnauthorized: false, keepAlive: true, maxSockets: CONCURRENCY })

function req(path) {
  return new Promise((resolve) => {
    const started = Date.now()
    const r = https.get(
      { host: '127.0.0.1', port: 443, path, servername: DOMAIN, agent,
        headers: { Host: DOMAIN, 'Accept-Language': 'zh-CN',
          'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/140.0 Safari/537.36' } },
      (res) => {
        let n = 0
        res.on('data', (c) => (n += c.length))
        res.on('end', () => resolve({ code: res.statusCode, ms: Date.now() - started, bytes: n }))
      }
    )
    r.on('error', (e) => resolve({ code: 'ERR:' + e.code, ms: Date.now() - started, bytes: 0 }))
    r.setTimeout(30000, () => { r.destroy(); resolve({ code: 'TIMEOUT', ms: Date.now() - started, bytes: 0 }) })
  })
}

function pct(sorted, p) {
  if (sorted.length === 0) return 0
  const i = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))
  return sorted[i]
}

async function runBatch(name, path) {
  const results = []
  const t0 = Date.now()
  await Promise.all(
    Array.from({ length: CONCURRENCY }, async () => {
      for (let i = 0; i < PER_WORKER; i++) results.push(await req(path))
    })
  )
  const wall = Date.now() - t0
  const codes = {}
  for (const r of results) codes[r.code] = (codes[r.code] || 0) + 1
  const times = results.map((r) => r.ms).sort((a, b) => a - b)
  const total = results.length
  const fivexx = results.filter((r) => typeof r.code === 'number' && r.code >= 500).length
  const errs = results.filter((r) => typeof r.code !== 'number').length

  console.log(`\n--- ${name}  (${path}) ---`)
  console.log(`  请求数 ${total} / 并发 ${CONCURRENCY} / 墙钟 ${wall}ms → ${Math.round((total / wall) * 1000)} req/s`)
  console.log(`  状态码分布: ${JSON.stringify(codes)}`)
  console.log(`  延迟 p50=${pct(times, 50)}ms  p90=${pct(times, 90)}ms  p95=${pct(times, 95)}ms  p99=${pct(times, 99)}ms  max=${times[times.length - 1]}ms`)
  console.log(`  平均响应体 ${Math.round(results.reduce((s, r) => s + r.bytes, 0) / total)} bytes`)
  return { name, total, fivexx, errs, p50: pct(times, 50), p95: pct(times, 95), p99: pct(times, 99), max: times[times.length - 1] }
}

;(async () => {
  console.log(`\n=== Batch-13 压测 @ ${DOMAIN} ===`)
  console.log(`（${CONCURRENCY} 并发 × ${PER_WORKER} 请求/并发）`)

  // 预热，避免把首连开销算进统计
  await req('/api/status')

  const out = []
  out.push(await runBatch('新增统计端点', '/api/site/visitors'))
  out.push(await runBatch('SEO 注入页（含价目表渲染）', '/pricing'))
  out.push(await runBatch('首页', '/'))

  console.log('\n=== 判定 ===')
  let bad = 0
  for (const r of out) {
    const ok = r.fivexx === 0 && r.errs === 0
    if (!ok) bad++
    console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${r.name}: 5xx=${r.fivexx} 连接错误=${r.errs} p95=${r.p95}ms p99=${r.p99}ms`)
  }
  console.log(`\n${bad === 0 ? '全部通过' : bad + ' 项失败'}\n`)
  process.exit(bad === 0 ? 0 : 1)
})().catch((e) => { console.error(e); process.exit(1) })
