/*
Audit helper: find `t('literal')` keys used in source that are missing from
en.json. Node-only, no deps. Run from web/:  node scripts/check-missing-i18n.cjs

Exits 1 when a real key is missing (usable as a CI gate). Dynamic template
keys (`` `Event.${x}` ``), numeric/brand literals are filtered as false
positives: i18next resolves those against a runtime-composed key, not en.json.
*/
const fs = require('node:fs')
const path = require('node:path')

const en = JSON.parse(
  fs.readFileSync('src/i18n/locales/en.json', 'utf8')
).translation
const known = new Set(Object.keys(en))

// Literals that are not translation keys (brand names, numbers, or the static
// prefix of a runtime-composed key). Keep this list tiny and justified.
const IGNORE = new Set(['0', '1', 'lza6', 'new-api-Max'])

const files = []
const walk = (d) => {
  for (const e of fs.readdirSync(d, { withFileTypes: true })) {
    const f = path.join(d, e.name)
    if (e.isDirectory()) {
      if (!/node_modules|__tests__|\.git/.test(e.name)) walk(f)
    } else if (/\.(tsx?|jsx?)$/.test(e.name)) files.push(f)
  }
}
walk('src')

const QUOTE = "['\"`]"
const re = new RegExp(`\\bt\\(\\s*${QUOTE}((?:[^'\"\`\\\\]|\\\\.)*)${QUOTE}\\s*[,)]`, 'g')
const missing = new Map()
for (const f of files) {
  if (/i18n|\.d\.ts/.test(f)) continue
  const src = fs.readFileSync(f, 'utf8')
  let m
  while ((m = re.exec(src))) {
    const k = m[1]
    // Skip dynamic/compound keys (`Event.${type}`) and known non-keys.
    if (!k || k.includes('${') || IGNORE.has(k) || known.has(k)) continue
    if (!missing.has(k)) missing.set(k, [])
    missing.get(k).push(f.replace('src/', ''))
  }
}

if (missing.size === 0) {
  console.log('i18n missing-key check: OK (0 missing)')
  process.exit(0)
}
console.error(`i18n missing-key check: FAILED (${missing.size} real keys missing)`)
for (const [k, fs] of [...missing.entries()].sort()) {
  console.error('  •', JSON.stringify(k), '→', [...new Set(fs)].slice(0, 3).join(', '))
}
process.exit(1)
