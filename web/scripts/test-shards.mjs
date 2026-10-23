#!/usr/bin/env node
/*
Frontend test shard runner (T-A1).

`vitest run` on the whole suite does not finish in one process on this repo
(single box, 197 test files, jsdom + i18n + MSW setup). Historically the suite
was split by hand into 5 batches and the numbers lived only in a planning doc.
This script makes the shards reproducible and prints one summary, so "did the
frontend suite pass?" becomes a single command with a numeric answer.

What it does
  1. Enumerates every `.test.ts` / `.test.tsx` file under src.
  2. Splits them into N shards, greedy largest-first (file size as a proxy for
     runtime), so shards are balanced without hardcoding names.
  3. Runs `vitest run` once per shard with the JSON reporter, sequentially.
  4. Prints per-shard totals and a grand total; exits 1 if any test failed or
     any shard crashed.

Deliberate non-goals
  - It never retries a failing test and never filters a failure out. A shard
    that fails makes the whole command fail.
  - It does not touch vitest.config.ts, so `bun run test` keeps its meaning.

Usage (from web/):
  bun scripts/test-shards.mjs                # default 5 shards
  bun scripts/test-shards.mjs --shards 3
  bun scripts/test-shards.mjs --dry-run      # show composition, run nothing
  bun scripts/test-shards.mjs --json         # machine-readable summary
*/
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

// fileURLToPath (not URL.pathname) so non-ASCII repo paths stay readable.
const WEB_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const SRC_ROOT = path.join(WEB_ROOT, 'src')
const TEST_FILE_RE = /\.test\.tsx?$/

function parseArgs(argv) {
  const opts = { shards: 5, dryRun: false, json: false }
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    if (arg === '--dry-run') opts.dryRun = true
    else if (arg === '--json') opts.json = true
    else if (arg === '--shards') {
      const value = Number(argv[++i])
      if (!Number.isInteger(value) || value < 1) {
        throw new Error(`--shards expects a positive integer, got ${argv[i]}`)
      }
      opts.shards = value
    } else {
      throw new Error(`unknown argument: ${arg}`)
    }
  }
  return opts
}

/** Depth-first walk, skipping build output and dependencies. */
function findTestFiles(dir) {
  const found = []
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist' || entry.name === 'coverage') continue
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      found.push(...findTestFiles(full))
    } else if (TEST_FILE_RE.test(entry.name)) {
      found.push({ file: full, size: fs.statSync(full).size })
    }
  }
  return found
}

/** Greedy largest-first assignment onto the currently lightest shard. */
function buildShards(files, shardCount) {
  const sorted = [...files].sort((a, b) => b.size - a.size)
  const shards = Array.from({ length: shardCount }, () => ({ load: 0, files: [] }))
  for (const item of sorted) {
    let target = shards[0]
    for (const shard of shards) {
      if (shard.load < target.load) target = shard
    }
    target.files.push(item.file)
    target.load += item.size
  }
  return shards.map((shard) => ({
    files: shard.files.sort(),
    bytes: shard.load,
  }))
}

function runShard(shard, index, total) {
  const outputFile = path.join(os.tmpdir(), `vitest-shard-${process.pid}-${index}.json`)
  if (fs.existsSync(outputFile)) fs.unlinkSync(outputFile)

  const relative = shard.files.map((file) => path.relative(WEB_ROOT, file))
  console.log(`\n── shard ${index + 1}/${total}: ${relative.length} files ──`)

  const result = spawnSync(
    process.platform === 'win32' ? 'bunx.cmd' : 'bunx',
    ['vitest', 'run', '--reporter=json', `--outputFile=${outputFile}`, ...relative],
    { cwd: WEB_ROOT, stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', shell: process.platform === 'win32' }
  )

  let summary = { total: 0, passed: 0, failed: 0, suites: 0, failures: [] }
  if (fs.existsSync(outputFile)) {
    try {
      const raw = JSON.parse(fs.readFileSync(outputFile, 'utf8'))
      // A runner that only says "3 failed" is not actionable: name them, with the
      // first line of the assertion message, so the next person can jump straight in.
      const failures = []
      for (const file of raw.testResults ?? []) {
        for (const assertion of file.assertionResults ?? []) {
          if (assertion.status === 'failed') {
            const firstLine = (assertion.failureMessages?.[0] ?? '')
              .split('\n')
              .find((line) => line.trim().length > 0) ?? '(no message)'
            failures.push({
              file: path.relative(WEB_ROOT, file.name ?? ''),
              name: assertion.fullName ?? assertion.title ?? '(unnamed)',
              message: firstLine.trim().slice(0, 200),
            })
          }
        }
      }
      summary = {
        total: raw.numTotalTests ?? 0,
        passed: raw.numPassedTests ?? 0,
        failed: raw.numFailedTests ?? 0,
        suites: raw.numTotalTestSuites ?? 0,
        failures,
      }
    } catch (error) {
      console.error(`  (could not parse JSON report: ${error.message})`)
    }
    fs.unlinkSync(outputFile)
  }

  const crashed = result.status !== 0 && summary.total === 0
  if (summary.total === 0) {
    const tail = `${result.stdout ?? ''}${result.stderr ?? ''}`.trim().split('\n').slice(-15).join('\n')
    console.error(tail)
  }
  console.log(
    `  tests ${summary.passed}/${summary.total} passed, ${summary.failed} failed; exit ${result.status}` +
      (crashed ? ' (no JSON report — shard crashed)' : '')
  )
  for (const failure of summary.failures) {
    console.error(`    ✗ ${failure.file} :: ${failure.name}`)
    console.error(`      ${failure.message}`)
  }

  return { index: index + 1, ...summary, exitCode: result.status, crashed }
}

function main() {
  const opts = parseArgs(process.argv.slice(2))
  const files = findTestFiles(SRC_ROOT)
  if (files.length === 0) {
    console.error('no test files found under src/')
    process.exit(2)
  }
  const shards = buildShards(files, opts.shards)

  if (opts.dryRun) {
    shards.forEach((shard, i) => {
      console.log(`shard ${i + 1}: ${shard.files.length} files, ${(shard.bytes / 1024).toFixed(0)} KiB`)
      for (const file of shard.files) console.log(`  ${path.relative(WEB_ROOT, file)}`)
    })
    console.log(`\n${files.length} test files -> ${opts.shards} shards (dry run, nothing executed)`)
    process.exit(0)
  }

  console.log(`${files.length} test files across ${opts.shards} shards`)
  const results = []
  const startedAt = Date.now()
  for (let i = 0; i < shards.length; i++) {
    results.push(runShard(shards[i], i, shards.length))
  }
  const elapsed = ((Date.now() - startedAt) / 1000).toFixed(1)

  const grand = results.reduce(
    (acc, r) => ({
      total: acc.total + r.total,
      passed: acc.passed + r.passed,
      failed: acc.failed + r.failed,
    }),
    { total: 0, passed: 0, failed: 0 }
  )
  const badShards = results.filter((r) => r.exitCode !== 0)

  if (opts.json) {
    console.log(JSON.stringify({ shards: results, grand, elapsedSeconds: Number(elapsed), ok: badShards.length === 0 }, null, 2))
  } else {
    console.log('\n════ summary ════')
    for (const r of results) {
      console.log(`shard ${r.index}: ${r.passed}/${r.total} passed, ${r.failed} failed${r.exitCode === 0 ? '' : `  [exit ${r.exitCode}]`}`)
    }
    console.log(`TOTAL: ${grand.passed}/${grand.total} passed, ${grand.failed} failed in ${elapsed}s`)
    console.log(badShards.length === 0 ? 'RESULT: PASS' : `RESULT: FAIL (${badShards.length} shard(s) not clean)`)
  }

  process.exit(badShards.length === 0 ? 0 : 1)
}

main()
