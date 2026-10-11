// Batch-13 站点访客统计的真实 E2E（真实浏览器 UA + 真实 HTTP + 真实数据库）。
//
// 验证的是用户原话里的两条硬要求：
//   「访问人数按照 ip 来算」+「一个 ip 一天只统计一次」+「显示的数据要真实的」
//
// 手法：同一个 IP 用不同 UA 连续访问首页若干次，然后**直接查数据库**确认
// 只有一行且 hits 累加；爬虫 UA 访问后确认不增加访客数。
//
// 用法：node visitor-e2e.cjs [baseUrl] [dbFile]
const fs = require('node:fs')

const BASE = process.argv[2] || 'http://127.0.0.1:3099'
const DB = process.argv[3] || null

const CHROME_LIKE =
  'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36'

let pass = 0
let fail = 0
function check(name, ok, detail) {
  if (ok) {
    pass++
    console.log(`  PASS  ${name}`)
  } else {
    fail++
    console.log(`  FAIL  ${name}${detail ? ' :: ' + detail : ''}`)
  }
}

async function getJson(url) {
  const res = await fetch(url, { headers: { 'User-Agent': CHROME_LIKE } })
  return { status: res.status, body: await res.text() }
}

/** 用指定 UA/头访问一次页面（每次 Connection: close，避免连接复用干扰）。 */
async function hit(url, { ua, referer } = {}) {
  const headers = { 'User-Agent': ua || CHROME_LIKE, Connection: 'close' }
  if (referer) headers.Referer = referer
  const res = await fetch(url, { headers, redirect: 'manual' })
  await res.text()
  return res.status
}

/**
 * 直接读 SQLite 校验落库结果。
 *
 * 用 Node 内置的 `node:sqlite`（Node ≥ 22）而不是 `sqlite3` CLI：
 * 本机没有装 sqlite3 命令行，而内置模块无需额外依赖 —— 少一个环境前提，
 * 脚本就更容易在别人机器上跑起来。
 */
function openDb(dbFile) {
  if (!DB || !fs.existsSync(dbFile || DB)) return null
  try {
    const { DatabaseSync } = require('node:sqlite')
    return new DatabaseSync(dbFile || DB, { readOnly: true })
  } catch (e) {
    console.log(`  [warn] 无法打开 DB 校验（${e.message}）—— 跳过数据库层断言`)
    return null
  }
}

/**
 * 轮询等待落库。
 *
 * 落库是**每 30 秒一批**（`service.visitorFlushEvery`），不是同步写 ——
 * 这是刻意的设计（避免把展示用统计变成最大的写库来源）。所以 E2E 不能固定
 * sleep 2 秒就断言，必须轮询到数据出现或超时，否则会得到一个假的 FAIL。
 */
/**
 * 等到一轮落库跑完。
 *
 * 落库周期是 30s（service.visitorFlushEvery），且**只在有待写数据时**才会真正写库。
 * 「不增加访客」这类断言如果不先等一轮，就会因为「什么都没落库」而**假 PASS** ——
 * 那正是伪验证。所以这里显式等过一个完整周期（+2s 余量）。
 */
const settle = () => new Promise((r) => setTimeout(r, 32000))

async function waitForFlush(timeoutMs = 45000) {
  const db = openDb(DB)
  if (!db) return null
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const rows = db
      .prepare('SELECT day, ip_hash, hits, source, path, is_bot FROM site_daily_visitors WHERE is_bot = 0 ORDER BY hits DESC')
      .all()
    if (rows.length > 0) {
      db.close()
      return rows
    }
    await new Promise((r) => setTimeout(r, 2000))
  }
  const all = db.prepare('SELECT COUNT(*) AS n FROM site_daily_visitors').get()
  db.close()
  console.log(`  [warn] 等待落库超时 ${timeoutMs}ms（库中总行数=${all.n}）`)
  return []
}

;(async () => {
  console.log(`\n=== Batch-13 访客统计 E2E @ ${BASE} ===\n`)

  console.log('[1] 公开统计端点可访问且结构完整')
  const r1 = await getJson(`${BASE}/api/site/visitors`)
  check('GET /api/site/visitors -> 200', r1.status === 200, `status=${r1.status}`)
  let payload = null
  try {
    payload = JSON.parse(r1.body)
  } catch {
    check('响应是 JSON', false, r1.body.slice(0, 200))
  }
  if (payload) {
    check('success=true', payload.success === true)
    const d = payload.data || {}
    const v = d.visitors || {}
    check('含 visitors 对象', typeof v === 'object')
    for (const k of ['today', 'last_7d', 'last_30d', 'total', 'page_views_today', 'page_views_total']) {
      check(`visitors.${k} 是数字`, typeof v[k] === 'number', `got ${typeof v[k]}`)
    }
    check('含 sources 数组', Array.isArray(d.sources), JSON.stringify(d.sources))
    check('含 registered_users 数字', typeof d.registered_users === 'number')
    check('含 active_users_24h 数字', typeof d.active_users_24h === 'number')
    check('含 active_users_30d 数字', typeof d.active_users_30d === 'number')
    check('含 generated_at', typeof d.generated_at === 'number' && d.generated_at > 0)
    check(
      '不含任何 IP/哈希/用户标识（隐私）',
      !/ip_hash|"\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}"|user_id|username/.test(r1.body),
      '响应体泄漏了可定位到个人的字段'
    )
    console.log(
      `     今日访客=${v.today}  近7天=${v.last_7d}  近30天=${v.last_30d}  累计=${v.total}  ` +
        `PV今日=${v.page_views_today}  PV累计=${v.page_views_total}  注册用户=${d.registered_users}`
    )
    if (Array.isArray(d.sources) && d.sources.length) {
      const total = d.sources.reduce((s, x) => s + x.count, 0)
      console.log(
        '     来源分布：' +
          d.sources
            .map((s) => `${s.source}=${s.count}(${((s.count / total) * 100).toFixed(1)}%)`)
            .join(' ')
      )
    }
  }

  const todayBefore = payload ? payload.data.visitors.today : 0

  console.log('\n[2] 同一 IP 多次访问只计一次（用户核心要求）')
  for (let i = 0; i < 5; i++) {
    await hit(`${BASE}/`)
    await hit(`${BASE}/pricing`)
  }
  // 落库是每 30 秒一批，必须轮询等待（见 waitForFlush 的说明）。
  const rows = await waitForFlush()
  if (rows === null) {
    console.log('     [skip] 未提供 DB 路径，跳过数据库层断言')
  } else if (rows.length === 0) {
    check('同一 IP 的多次访问落在同一行', false, '等待落库超时且库中无行')
  } else {
    console.log(`     库中非爬虫行数 = ${rows.length}`)
    rows.slice(0, 5).forEach((x) => {
      console.log(
        `       day=${x.day} ip_hash=${String(x.ip_hash).slice(0, 10)}… hits=${x.hits} source=${x.source} path=${x.path}`
      )
    })
    const sameIpRows = rows.filter((x) => x.hits > 1).length
    check(
      '同一 IP 的多次访问落在同一行（hits 累加而不是新增行）',
      sameIpRows >= 1,
      `hits>1 的行数=${sameIpRows}，总行数=${rows.length}`
    )
    const uniq = new Set(rows.map((x) => x.day + '|' + x.ip_hash)).size
    check('(day, ip_hash) 组合唯一', uniq === rows.length, `唯一组合=${uniq} 行数=${rows.length}`)
  }

  const after = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data
  console.log(`     今日访客 before=${todayBefore} after=${after.visitors.today}`)
  // 本地实例用同一个回环 IP 访问，所以今日访客增量最多 1。
  check(
    '同一 IP 只贡献 1 个访客（增量 ≤ 1）',
    after.visitors.today - todayBefore <= 1,
    `增量=${after.visitors.today - todayBefore}`
  )

  console.log('\n[3] 爬虫不计入访客')
  const beforeBot = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data.visitors.today
  await hit(`${BASE}/`, {
    ua: 'Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)',
  })
  await hit(`${BASE}/`, { ua: 'curl/8.4.0' })
  await hit(`${BASE}/`, { ua: 'python-requests/2.31.0' })
  await settle()
  const afterBot = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data.visitors.today
  check(
    'Googlebot / curl / python-requests 不增加今日访客',
    afterBot === beforeBot,
    `before=${beforeBot} after=${afterBot}`
  )

  console.log('\n[4] 静态资源与 API 不计入访客')
  const beforeStatic = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data.visitors.today
  await hit(`${BASE}/api/status`)
  await hit(`${BASE}/logo.png`)
  await hit(`${BASE}/favicon.ico`)
  await hit(`${BASE}/static/js/index.js`)
  await settle()
  const afterStatic = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data.visitors.today
  check(
    '/api/* 与静态资源不计入访客',
    afterStatic === beforeStatic,
    `before=${beforeStatic} after=${afterStatic}`
  )

  console.log('\n[5] 来源归类（带 Referer 的访问）')
  await hit(`${BASE}/`, { referer: 'https://www.google.com/search?q=ai+api' })
  await hit(`${BASE}/`, { referer: 'https://twitter.com/x/status/1' })
  await hit(`${BASE}/`, { referer: 'https://someblog.example/post' })
  await settle()
  const src = JSON.parse((await getJson(`${BASE}/api/site/visitors`)).body).data.sources
  console.log('     sources = ' + JSON.stringify(src))
  check('来源分布非空且每项含 source/count', Array.isArray(src) && src.every((s) => s.source && typeof s.count === 'number'))

  console.log(`\n=== 结果：${pass} PASS / ${fail} FAIL ===\n`)
  process.exit(fail === 0 ? 0 : 1)
})()

