/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * knip-gate — "no NEW dead code" gate (§4.2.5).
 *
 * Runs `knip --reporter json` and compares the resulting dead-code issue keys
 * against the checked-in baseline (scripts/knip-baseline.json). Any issue key
 * NOT present in the baseline is treated as newly introduced dead code and
 * fails the gate (exit 1). The existing 600+ legacy backlog is deliberately
 * out of scope: baseline issues never block CI.
 *
 * Modes:
 *   node scripts/knip-gate.mjs            — gate: exit 1 on new dead code
 *   node scripts/knip-gate.mjs --update   — write current issues into the baseline
 *
 * Cross-platform: pure Node ESM, runs knip through the same Node runtime
 * (`process.execPath`) from the web/ package root, no shell-specific syntax.
 */
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = join(__dirname, '..')
const KNIP_BIN = join(WEB_ROOT, 'node_modules', 'knip', 'bin', 'knip.js')
const BASELINE_FILE = join(__dirname, 'knip-baseline.json')

// Categories reported by `knip --reporter json`. Kept aligned with knip 6.x;
// unknown future categories are ignored safely (missing -> empty array).
const CATEGORIES = [
  'binaries',
  'catalog',
  'dependencies',
  'devDependencies',
  'duplicates',
  'enumMembers',
  'exports',
  'files',
  'namespaceMembers',
  'optionalPeerDependencies',
  'types',
  'unlisted',
  'unresolved',
]

/**
 * Normalize knip issues into a stable set of "category:file:symbol" keys.
 * - Most categories carry objects: { name, line, col, pos }.
 * - `duplicates` items are arrays of such objects (a group of duplicate
 *   exports) and are flattened.
 * - File-granular categories (e.g. `files`) yield `category:file:name`
 *   where name === the file itself; still stable for baseline diffing.
 */
function issueKeys(issues) {
  const keys = new Set()
  for (const issue of issues) {
    const file = issue.file ?? ''
    for (const category of CATEGORIES) {
      const items = issue[category]
      if (!Array.isArray(items)) continue
      for (const item of items) {
        const group = Array.isArray(item) ? item : [item]
        for (const entry of group) {
          keys.add(`${category}:${file}:${entry?.name ?? ''}`)
        }
      }
    }
  }
  return keys
}

function runKnip() {
  // knip exits 1 when issues exist but still prints the JSON report on stdout.
  try {
    return execFileSync(process.execPath, [KNIP_BIN, '--reporter', 'json'], {
      cwd: WEB_ROOT,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    })
  } catch (error) {
    if (error && typeof error.stdout === 'string' && error.stdout.length > 0) {
      return error.stdout
    }
    throw error
  }
}

function parseReport(stdout) {
  let data
  try {
    data = JSON.parse(stdout)
  } catch (error) {
    throw new Error(`knip --reporter json produced unparseable output: ${String(error)}`)
  }
  if (!Array.isArray(data?.issues)) {
    throw new Error('knip --reporter json produced an unexpected shape: "issues" array missing')
  }
  return issueKeys(data.issues)
}

function main() {
  const update = process.argv.includes('--update')
  const current = parseReport(runKnip())

  if (update) {
    const sorted = [...current].sort()
    writeFileSync(BASELINE_FILE, `${JSON.stringify(sorted, null, 2)}\n`)
    console.log(`knip-gate: baseline updated: ${sorted.length} issue keys -> ${BASELINE_FILE}`)
    process.exit(0)
  }

  if (!existsSync(BASELINE_FILE)) {
    console.error(
      `knip-gate: baseline file not found (${BASELINE_FILE}). ` +
        `Run "node scripts/knip-gate.mjs --update" to establish it first.`
    )
    process.exit(2)
  }

  let baseline
  try {
    baseline = new Set(JSON.parse(readFileSync(BASELINE_FILE, 'utf8')))
  } catch (error) {
    console.error(`knip-gate: failed to parse baseline ${BASELINE_FILE}: ${String(error)}`)
    process.exit(2)
  }

  const added = [...current].filter((key) => !baseline.has(key)).sort()
  if (added.length > 0) {
    console.error(`knip-gate: ${added.length} NEW dead-code issue(s) not present in baseline:`)
    for (const key of added) console.error(`  ${key}`)
    console.error('Run "node scripts/knip-gate.mjs --update" only if they are intentional (do not mask regressions).')
    process.exit(1)
  }

  console.log(`knip-gate: ok — ${current.size} dead-code issue(s), none new vs baseline (${baseline.size} keys)`)
  process.exit(0)
}

main()