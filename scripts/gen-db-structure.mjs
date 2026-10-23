#!/usr/bin/env node
/*
db_structure.md generator (T-A8).

`db_structure.md` is the authoritative structural reference for the project, so
it must never be hand-written from memory — it is generated from a real database
that the application itself migrated. Hand-copied schemas drift within a week.

Source of truth = the schema GORM creates when the gateway boots against a
throwaway SQLite file. SQLite is used because it needs no service; the script
records that caveat, and notes where MySQL/PostgreSQL differ.

Recipe (documented in the generated file header):

  go build -o /tmp/new-api-dump .
  SQLITE_PATH=/tmp/spec.db PORT=3311 SESSION_SECRET=dump ./new-api-dump &   # boots + AutoMigrate
  node scripts/gen-db-structure.mjs --db /tmp/spec.db --out db_structure.md

Usage:
  node scripts/gen-db-structure.mjs --db <sqlite-file> [--out db_structure.md] [--json]
*/
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { DatabaseSync } from 'node:sqlite'

const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

function parseArgs(argv) {
  const opts = { db: null, out: path.join(REPO_ROOT, 'db_structure.md'), json: false }
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === '--db') opts.db = argv[++i]
    else if (argv[i] === '--out') opts.out = argv[++i]
    else if (argv[i] === '--json') opts.json = true
    else throw new Error(`unknown argument: ${argv[i]}`)
  }
  if (!opts.db) throw new Error('--db <sqlite-file> is required')
  if (!fs.existsSync(opts.db)) throw new Error(`database file not found: ${opts.db}`)
  return opts
}

function readSchema(dbPath) {
  const db = new DatabaseSync(dbPath, { readOnly: true })
  try {
    const tableRows = db
      .prepare(
        "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
      )
      .all()

    const tables = tableRows.map((row) => {
      const columns = db.prepare(`PRAGMA table_info("${row.name}")`).all().map((c) => ({
        name: c.name,
        type: c.type || '(untyped)',
        notNull: Boolean(c.notnull),
        primaryKey: Boolean(c.pk),
        default: c.dflt_value,
      }))
      const indexes = db.prepare(`PRAGMA index_list("${row.name}")`).all().map((i) => ({
        name: i.name,
        unique: Boolean(i.unique),
        columns: db
          .prepare(`PRAGMA index_info("${i.name}")`)
          .all()
          .map((c) => c.name),
      }))
      const { c: rowCount } = db.prepare(`SELECT COUNT(*) AS c FROM "${row.name}"`).get()
      return { name: row.name, rowCount, columns, indexes }
    })
    return tables
  } finally {
    db.close()
  }
}

function renderMarkdown(tables, meta) {
  const totalColumns = tables.reduce((acc, t) => acc + t.columns.length, 0)
  const lines = []
  lines.push('# db_structure.md — 数据库结构权威参考')
  lines.push('')
  lines.push('> **本文件由脚本生成，禁止手改。** 手抄 schema 一定漂移；改结构请改 `model/` 与迁移代码，')
  lines.push('> 然后按下方命令重新生成并提交本文件。')
  lines.push('')
  lines.push('## 0. 权威关系声明')
  lines.push('')
  lines.push('| 问题 | 权威来源 |')
  lines.push('|---|---|')
  lines.push('| 表 / 字段 / 索引 / 约束 | **本文件**（由真实 AutoMigrate 结果生成） |')
  lines.push('| 业务含义与字段语义 | `model/*.go` 的注释与校验逻辑 |')
  lines.push('| 当前任务状态 | `project_specs.md` |')
  lines.push('| 线上运行版本 | `workflow_status.md` §0 |')
  lines.push('| 验证记录 / 噪声 | `test_ledger.md`、`计划书/audit-ledger.md` |')
  lines.push('')
  lines.push('## 1. 生成方式（可复现）')
  lines.push('')
  lines.push('```bash')
  lines.push('# 1) 构建（不注入版本，只为跑一次迁移）')
  lines.push('go build -o /tmp/new-api-dump .')
  lines.push('# 2) 用一次性 SQLite 启动，触发 AutoMigrate（数据不落生产）')
  lines.push('SQLITE_PATH=/tmp/spec.db PORT=3311 SESSION_SECRET=dump GIN_MODE=release /tmp/new-api-dump &')
  lines.push('# 3) 生成')
  lines.push('node scripts/gen-db-structure.mjs --db /tmp/spec.db --out db_structure.md')
  lines.push('```')
  lines.push('')
  lines.push('## 2. 本次生成事实')
  lines.push('')
  lines.push('| 项 | 值 |')
  lines.push('|---|---|')
  lines.push(`| 生成时间 | ${meta.generatedAt} |`)
  lines.push(`| 引擎 | SQLite（AutoMigrate 真实执行） |`)
  lines.push(`| 表数量 | ${tables.length} |`)
  lines.push(`| 字段总数 | ${totalColumns} |`)
  lines.push(`| 生成命令 | \`${meta.command}\` |`)
  lines.push(`| 源文件 | \`${path.basename(meta.dbPath)}\`（一次性库，非生产） |`)
  lines.push('')
  lines.push('**已知边界（必须知道，否则会误判）**')
  lines.push('')
  lines.push('1. SQLite 是「无服务即可跑」的代表方言，**不保证** MySQL / PostgreSQL 的类型字节级一致：')
  lines.push('   例如 `bool` 在 SQLite 是整数、在 PG 是真布尔；`text` 长度限制由 GORM tag 决定，SQLite 不强制。')
  lines.push('2. 索引名在各方言可能被截断或加后缀；本文件记录的是 SQLite 的实际命名。')
  lines.push('3. 本文件只覆盖**主库**。日志库（`LOG_SQL_DSN`，可指向 ClickHouse）结构不同，见 §4。')
  lines.push('4. 任何 schema 变更都必须按 `AGENTS.md` 的三库验证要求跑 `make db-check`。')
  lines.push('')
  lines.push('## 3. 表清单')
  lines.push('')
  lines.push('| # | 表 | 字段数 | 索引数 | 本次迁移后行数 |')
  lines.push('|---|---|---|---|---|')
  tables.forEach((t, i) => {
    lines.push(`| ${i + 1} | \`${t.name}\` | ${t.columns.length} | ${t.indexes.length} | ${t.rowCount} |`)
  })
  lines.push('')
  lines.push('## 4. 逐表结构')
  lines.push('')
  for (const t of tables) {
    lines.push(`### \`${t.name}\``)
    lines.push('')
    lines.push('| 字段 | 类型 | NOT NULL | 主键 | 默认值 |')
    lines.push('|---|---|---|---|---|')
    for (const c of t.columns) {
      lines.push(
        `| \`${c.name}\` | ${c.type} | ${c.notNull ? 'yes' : ''} | ${c.primaryKey ? 'yes' : ''} | ${
          c.default === null || c.default === undefined ? '' : `\`${c.default}\``
        } |`
      )
    }
    lines.push('')
    if (t.indexes.length > 0) {
      lines.push('索引：')
      for (const idx of t.indexes) {
        const cols = idx.columns.filter(Boolean).join(', ')
        lines.push(`- \`${idx.name}\`${idx.unique ? '（UNIQUE）' : ''} on (${cols})`)
      }
      lines.push('')
    }
  }
  lines.push('## 5. 日志库')
  lines.push('')
  lines.push('日志表由 `LOG_SQL_DSN` 指向的独立库承载（支持 SQLite / MySQL / PostgreSQL / ClickHouse）。')
  lines.push('其结构不在本文件范围内：**从未验证过 ClickHouse 这条路径**（见 `计划书/下一步改进指南.md` 任务卡 T-A4）。')
  lines.push('')
  return `${lines.join('\n')}\n`
}

function main() {
  const opts = parseArgs(process.argv.slice(2))
  const tables = readSchema(opts.db)
  const meta = {
    generatedAt: new Date().toISOString(),
    dbPath: opts.db,
    command: `node scripts/gen-db-structure.mjs --db ${path.basename(opts.db)} --out ${path.basename(opts.out)}`,
  }

  if (opts.json) {
    console.log(JSON.stringify({ meta, tableCount: tables.length, tables }, null, 2))
    return
  }

  const markdown = renderMarkdown(tables, meta)
  fs.writeFileSync(opts.out, markdown, 'utf8')
  const totalColumns = tables.reduce((acc, t) => acc + t.columns.length, 0)
  console.log(`wrote ${opts.out}`)
  console.log(`  tables: ${tables.length}`)
  console.log(`  columns: ${totalColumns}`)
  console.log(`  bytes: ${markdown.length}`)
}

main()
