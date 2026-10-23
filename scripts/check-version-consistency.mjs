#!/usr/bin/env node
/*
Version consistency gate (T-A5).

The repo states the current version in several hand-maintained places. They have
drifted before (v1.3.181 shipped while 计划书/README.md still said "latest tag =
v1.3.180"), so this script compares them mechanically instead of trusting prose.

Sources compared
  1. VERSION                      — the release marker
  2. CHANGELOG.md first heading   — must be the same release
  3. git tag / git describe       — HEAD must carry that exact tag
  4. workflow_status.md §0        — deployed version (allowed to lag VERSION)
  5. 计划书/README.md 「最后核对」  — prose claim, must not contradict 1–3

Exit codes
  0  all sources agree (a lagging deployment is reported as a note, not a failure)
  1  at least one hard inconsistency
  2  a file could not be read / parsed

Usage (from the repo root or anywhere — paths resolve from the repo root):
  node scripts/check-version-consistency.mjs
  node scripts/check-version-consistency.mjs --self-test   # fabricated facts, no repo IO
  node scripts/check-version-consistency.mjs --json
*/
import { execFileSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const VERSION_RE = /^v\d+\.\d+\.\d+$/

/** Parse "v1.3.181" -> [1,3,181]; returns null when unparsable. */
function parseVersion(raw) {
  if (typeof raw !== 'string') return null
  const m = raw.trim().match(/^v?(\d+)\.(\d+)\.(\d+)$/)
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null
}

function compareVersion(a, b) {
  const pa = parseVersion(a)
  const pb = parseVersion(b)
  if (!pa || !pb) return 0
  for (let i = 0; i < 3; i++) {
    if (pa[i] !== pb[i]) return pa[i] - pb[i]
  }
  return 0
}

function readText(relativePath) {
  const abs = path.join(REPO_ROOT, relativePath)
  if (!fs.existsSync(abs)) {
    throw new Error(`missing file: ${relativePath}`)
  }
  return fs.readFileSync(abs, 'utf8')
}

function git(...args) {
  return execFileSync('git', args, { cwd: REPO_ROOT, encoding: 'utf8' }).trim()
}

/**
 * Collect the five sources. Kept separate from the checks so --self-test can
 * exercise the checks against fabricated facts without touching the repo.
 */
function collectFacts() {
  const facts = { problems: [] }

  const versionRaw = readText('VERSION').trim()
  facts.version = VERSION_RE.test(versionRaw) ? versionRaw : null
  facts.versionRaw = versionRaw

  const changelog = readText('CHANGELOG.md')
  const clMatch = changelog.match(/^##\s*\[(v\d+\.\d+\.\d+)\]/m)
  facts.changelogVersion = clMatch ? clMatch[1] : null

  try {
    facts.describeTag = git('describe', '--tags', '--abbrev=0')
  } catch {
    facts.describeTag = null
  }
  try {
    // Exact tag on HEAD, empty when HEAD is ahead of the newest tag.
    facts.exactTag = git('describe', '--tags', '--exact-match')
  } catch {
    facts.exactTag = null
  }

  const status = readText('workflow_status.md')
  const deployed = status.match(/^\|\s*线上版本\s*\|\s*`?(v\d+\.\d+\.\d+)`?/m)
  facts.deployedVersion = deployed ? deployed[1] : null

  let readmeSection = ''
  try {
    const readme = readText('计划书/README.md')
    const idx = readme.indexOf('最后核对')
    readmeSection = idx === -1 ? '' : readme.slice(idx, idx + 800)
  } catch {
    readmeSection = ''
  }
  const readmeClaims = [...readmeSection.matchAll(/v\d+\.\d+\.\d+/g)].map((m) => m[0])
  facts.readmeSectionFound = readmeSection.length > 0
  // Distinct claims: the README legitimately mentions the rollback version too,
  // so only the claim tied to `VERSION` / `最新 tag` is treated as authoritative.
  const versionClaim = readmeSection.match(/`VERSION`\s*=\s*`(v\d+\.\d+\.\d+)`/)
  const tagClaim = readmeSection.match(/最新 tag\s*=\s*`(v\d+\.\d+\.\d+)`/)
  facts.readmeVersionClaim = versionClaim ? versionClaim[1] : null
  facts.readmeTagClaim = tagClaim ? tagClaim[1] : null
  facts.readmeClaims = [...new Set(readmeClaims)]

  return facts
}

/** Pure check step: returns {errors, notes}. */
function checkFacts(facts) {
  const errors = []
  const notes = []

  if (!facts.version) {
    errors.push(`VERSION is not a vX.Y.Z release marker (got: ${JSON.stringify(facts.versionRaw)})`)
  }
  if (!facts.changelogVersion) {
    errors.push('CHANGELOG.md has no "## [vX.Y.Z]" heading')
  }
  if (facts.version && facts.changelogVersion && facts.version !== facts.changelogVersion) {
    errors.push(`VERSION (${facts.version}) != CHANGELOG first heading (${facts.changelogVersion})`)
  }
  if (facts.version && facts.describeTag && facts.version !== facts.describeTag) {
    // A version bump legitimately lands before its tag: VERSION/CHANGELOG move
    // first, the tag is created at release time. Only "tag is newer than
    // VERSION" is impossible. --require-tag makes the release pipeline strict.
    const delta = compareVersion(facts.describeTag, facts.version)
    if (delta > 0) {
      errors.push(
        `newest git tag (${facts.describeTag}) is newer than VERSION (${facts.version}) — tag exists for a version the repo does not declare`
      )
    } else if (facts.requireTag) {
      errors.push(`VERSION (${facts.version}) is not tagged yet (newest tag ${facts.describeTag}) and --require-tag was passed`)
    } else {
      notes.push(
        `VERSION (${facts.version}) is ahead of the newest tag (${facts.describeTag}) — pending release, tag it when publishing`
      )
    }
  }
  if (facts.version && !facts.exactTag) {
    if (facts.requireTag) {
      errors.push(`HEAD is not tagged ${facts.version} (--require-tag)`)
    } else {
      notes.push('HEAD is untagged (release not published yet)')
    }
  } else if (facts.version && facts.exactTag && facts.version !== facts.exactTag) {
    // HEAD still sits on the previous tag while VERSION already moved forward:
    // that is the normal window between "bump" and "publish".
    if (compareVersion(facts.exactTag, facts.version) < 0 && !facts.requireTag) {
      notes.push(`HEAD is still tagged ${facts.exactTag} while VERSION is ${facts.version} (pending release)`)
    } else {
      errors.push(`HEAD carries tag ${facts.exactTag}, VERSION says ${facts.version}`)
    }
  }

  // Deployment is allowed to lag, but never to run ahead of the repo marker.
  if (!facts.deployedVersion) {
    notes.push('workflow_status.md §0: no 线上版本 row found (skipped)')
  } else if (facts.version) {
    const delta = compareVersion(facts.deployedVersion, facts.version)
    if (delta > 0) {
      errors.push(
        `workflow_status.md §0 claims deployed ${facts.deployedVersion}, which is newer than VERSION ${facts.version}`
      )
    } else if (delta < 0) {
      notes.push(
        `workflow_status.md §0: deployed ${facts.deployedVersion} lags VERSION ${facts.version} (deployment may be pending)`
      )
    }
  }

  if (!facts.readmeSectionFound) {
    notes.push('计划书/README.md: 「最后核对」section not found (skipped)')
  } else {
    if (facts.readmeVersionClaim && facts.version && facts.readmeVersionClaim !== facts.version) {
      errors.push(
        `计划书/README.md 最后核对 claims VERSION = ${facts.readmeVersionClaim}, actual ${facts.version}`
      )
    }
    if (facts.readmeTagClaim && facts.describeTag && facts.readmeTagClaim !== facts.describeTag) {
      errors.push(
        `计划书/README.md 最后核对 claims 最新 tag = ${facts.readmeTagClaim}, actual ${facts.describeTag}`
      )
    }
    if (!facts.readmeVersionClaim && !facts.readmeTagClaim) {
      notes.push('计划书/README.md: 「最后核对」has no parsable VERSION/最新 tag claim (skipped)')
    }
  }

  return { errors, notes }
}

/** Fabricated facts -> expected exit behaviour. Proves the checks actually fire. */
function selfTest() {
  const good = {
    versionRaw: 'v1.3.181',
    version: 'v1.3.181',
    changelogVersion: 'v1.3.181',
    describeTag: 'v1.3.181',
    exactTag: 'v1.3.181',
    deployedVersion: 'v1.3.181',
    readmeSectionFound: true,
    readmeVersionClaim: 'v1.3.181',
    readmeTagClaim: 'v1.3.181',
    requireTag: true,
  }
  const cases = [
    ['consistent facts', good, 0],
    ['changelog lags VERSION', { ...good, changelogVersion: 'v1.3.180' }, 1],
    ['version bumped, tag pending (dev)', { ...good, requireTag: false, describeTag: 'v1.3.180', exactTag: 'v1.3.180', readmeTagClaim: 'v1.3.180' }, 0],
    ['version bumped, tag pending (release)', { ...good, describeTag: 'v1.3.180', exactTag: null }, 1],
    ['tag newer than VERSION (impossible)', { ...good, version: 'v1.3.181', versionRaw: 'v1.3.181', describeTag: 'v1.3.182', exactTag: 'v1.3.182', readmeTagClaim: 'v1.3.182' }, 1],
    ['readme claims old tag', { ...good, readmeTagClaim: 'v1.3.180' }, 1],
    ['deployed behind (allowed)', { ...good, deployedVersion: 'v1.3.180' }, 0],
    ['deployed ahead (impossible)', { ...good, deployedVersion: 'v1.3.182' }, 1],
    ['bad VERSION format', { ...good, versionRaw: '1.3.181', version: null }, 1],
  ]

  let failed = 0
  for (const [name, facts, expected] of cases) {
    const { errors } = checkFacts(facts)
    const actual = errors.length > 0 ? 1 : 0
    const ok = actual === expected
    if (!ok) failed++
    console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}  (exit ${actual}, expected ${expected})`)
    if (!ok) errors.forEach((e) => console.log(`        - ${e}`))
  }
  console.log(`\nself-test: ${cases.length - failed}/${cases.length} passed`)
  return failed === 0 ? 0 : 1
}

function main() {
  const argv = process.argv.slice(2)
  if (argv.includes('--self-test')) {
    process.exit(selfTest())
  }

  let facts
  try {
    facts = collectFacts()
  } catch (error) {
    console.error(`ERROR  ${error.message}`)
    process.exit(2)
  }
  facts.requireTag = argv.includes('--require-tag')

  const { errors, notes } = checkFacts(facts)

  if (argv.includes('--json')) {
    console.log(JSON.stringify({ facts, errors, notes }, null, 2))
  } else {
    console.log('Version sources')
    console.log(`  VERSION                       ${facts.versionRaw}`)
    console.log(`  CHANGELOG first heading       ${facts.changelogVersion ?? '(none)'}`)
    console.log(`  newest git tag                ${facts.describeTag ?? '(none)'}`)
    console.log(`  tag on HEAD                   ${facts.exactTag ?? '(untagged)'}`)
    console.log(`  workflow_status §0 deployed   ${facts.deployedVersion ?? '(no row)'}`)
    console.log(`  计划书/README 最后核对 claim   ${facts.readmeVersionClaim ?? facts.readmeTagClaim ?? '(none)'}`)
    for (const note of notes) console.log(`  note: ${note}`)
    console.log('')
  }

  if (errors.length === 0) {
    console.log('RESULT: consistent')
    process.exit(0)
  }
  console.error('RESULT: inconsistent')
  errors.forEach((e) => console.error(`  - ${e}`))
  process.exit(1)
}

main()
