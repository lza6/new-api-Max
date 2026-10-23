/*
Coverage audit: every non-English locale must translate every key that exists in
en.json (and en.common.json). The sibling check-missing-i18n.cjs only answers a
different question — "does every `t('literal')` used in source exist in en.json".
It never catches a locale that is simply missing translations, which is how
fr/ja/ru/vi ended up 95 keys short while both scripts reported OK.

Node-only, no deps. Run from web/: node scripts/check-i18n-coverage.cjs
Exits 1 when any locale is missing keys (usable as a CI gate).
*/
const fs = require('node:fs')
const path = require('node:path')

const LOCALES_DIR = path.join('src', 'i18n', 'locales')

const load = (file) => {
  const raw = fs.readFileSync(path.join(LOCALES_DIR, file), 'utf8')
  return JSON.parse(raw).translation
}

// English is the source of truth: keys are the English source strings.
const pairs = [
  { base: 'en.json', others: ['fr', 'ja', 'ru', 'vi', 'zh', 'zh-TW'] },
  { base: 'en.common.json', others: ['fr', 'ja', 'ru', 'vi', 'zh', 'zh-TW'] },
]

const fileFor = (lang, base) => (base === 'en.json' ? `${lang}.json` : `${lang}.common.json`)

// Interpolation placeholders (`{{name}}`, `{{count}}`, …) must survive
// translation. A lost placeholder silently shows a sentence with the value
// missing, and a placeholder that never existed renders as literal braces or an
// empty string — both were found in ja/vi before this check existed.
const PLACEHOLDERS = /\{\{\s*[\w.]+\s*\}\}/g
const placeholdersOf = (text) => {
  const found = String(text).match(PLACEHOLDERS) ?? []
  return [...new Set(found.map((token) => token.replace(/[{}\s]/g, '')))].sort()
}

const jsonMode = process.argv.includes('--json')
const report = []
let failed = false

for (const { base, others } of pairs) {
  const baseEntries = load(base)
  const baseKeys = Object.keys(baseEntries)
  for (const lang of others) {
    const file = fileFor(lang, base)
    if (!fs.existsSync(path.join(LOCALES_DIR, file))) {
      report.push({ file, missing: baseKeys.length, missingKeys: baseKeys, extra: 0, absent: true, placeholderIssues: [] })
      failed = true
      continue
    }
    const translated = load(file)
    const missingKeys = baseKeys.filter((k) => !(k in translated))
    const extra = Object.keys(translated).filter((k) => !baseKeys.includes(k)).length
    const placeholderIssues = []
    for (const key of baseKeys) {
      if (!(key in translated)) continue
      const want = placeholdersOf(baseEntries[key]).join(',')
      const have = placeholdersOf(translated[key]).join(',')
      if (want !== have) placeholderIssues.push({ key, want, have })
    }
    if (missingKeys.length || placeholderIssues.length) failed = true
    report.push({ file, missing: missingKeys.length, missingKeys, extra, placeholderIssues })
  }
}

if (jsonMode) {
  console.log(JSON.stringify(report, null, 2))
} else {
  for (const entry of report) {
    const label = `${entry.file}`.padEnd(20)
    if (entry.absent) {
      console.error(`✗ ${label} file is missing entirely`)
      continue
    }
    if (entry.missing) {
      console.error(`✗ ${label} ${entry.missing} keys missing (shows English via fallback)`)
      for (const k of entry.missingKeys.slice(0, 5)) console.error(`    • ${JSON.stringify(k)}`)
      if (entry.missing > 5) console.error(`    … ${entry.missing - 5} more`)
    }
    if (entry.placeholderIssues.length) {
      console.error(`✗ ${label} ${entry.placeholderIssues.length} placeholder mismatch(es)`)
      for (const issue of entry.placeholderIssues.slice(0, 5)) {
        console.error(`    • ${JSON.stringify(issue.key)}`)
        console.error(`        expected [${issue.want}] got [${issue.have}]`)
      }
      if (entry.placeholderIssues.length > 5) {
        console.error(`    … ${entry.placeholderIssues.length - 5} more`)
      }
    }
    if (!entry.missing && !entry.placeholderIssues.length) {
      const extraNote = entry.extra ? `, ${entry.extra} unused extra keys` : ''
      console.log(`✓ ${label} complete${extraNote}`)
    }
  }
}

if (failed) {
  console.error(
    '\ni18n coverage check FAILED. Add the missing translations (never delete en keys).'
  )
  process.exit(1)
}
console.log('\ni18n coverage check: OK (all locales translated)')
