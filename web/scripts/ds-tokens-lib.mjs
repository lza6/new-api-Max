/*
Design-token snapshot: shared parser for ds-tokens-export.mjs / ds-tokens-check.mjs.

Why this exists: theme.css and theme-presets.css are the single source of truth
for colour, radius, motion and typography tokens. Without a committed snapshot,
a token can be changed (or silently dropped) and nothing fails — the visual
guide (V1–V13) is being executed in parallel, so drift is a live risk.

The parser is intentionally small: it only extracts custom properties
(`--name: value`) per block, normalises whitespace, and sorts keys so that
re-generating from unchanged CSS is byte-identical. It is not a CSS parser and
does not need to be — anything that is not a custom property is ignored.
*/
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'

const SOURCES = ['src/styles/theme.css', 'src/styles/theme-presets.css']
export const SNAPSHOT_PATH = '.ai/ds/ds-tokens.json'

const stripComments = (css) => css.replace(/\/\*[\s\S]*?\*\//g, '')
const normalizeSelector = (raw) => raw.trim().replace(/\s+/g, ' ')
const normalizeValue = (raw) => raw.trim().replace(/\s+/g, ' ')

/** Split a stylesheet into blocks of declarations, keeping the selector path. */
const parseBlocks = (css) => {
  const blocks = []
  let i = 0

  const walk = (trail, nested) => {
    let buffer = ''
    const decls = []
    while (i < css.length) {
      const ch = css[i]
      if (ch === '{') {
        i += 1
        const selector = normalizeSelector(buffer)
        buffer = ''
        walk(selector ? [...trail, selector] : trail, true)
        continue
      }
      if (ch === '}' && nested) {
        i += 1
        break
      }
      if (ch === ';') {
        decls.push(buffer)
        buffer = ''
        i += 1
        continue
      }
      buffer += ch
      i += 1
    }
    if (buffer.trim()) decls.push(buffer)
    blocks.push({ trail, decls })
  }

  walk([], false)
  return blocks
}

/** Extract `--name: value` pairs from one block's declarations. */
const collectDecls = (decls) => {
  const tokens = {}
  for (const decl of decls) {
    const trimmed = decl.trim()
    if (!trimmed.startsWith('--')) continue
    const colon = trimmed.indexOf(':')
    if (colon === -1) continue
    const name = trimmed.slice(0, colon).trim()
    const value = normalizeValue(trimmed.slice(colon + 1))
    if (name.length > 2 && value) tokens[name] = value
  }
  return tokens
}

const hashFile = (cwd, file) =>
  crypto
    .createHash('sha256')
    .update(fs.readFileSync(path.join(cwd, file)))
    .digest('hex')
    .slice(0, 16)

/** Build the snapshot object. Keys are sorted, so output is deterministic. */
export const collectTokens = (cwd) => {
  const sources = []
  const blocks = []

  for (const file of SOURCES) {
    const abs = path.join(cwd, file)
    if (!fs.existsSync(abs)) continue
    sources.push({ file, sha256_16: hashFile(cwd, file) })
    const css = stripComments(fs.readFileSync(abs, 'utf8'))
    for (const { trail, decls } of parseBlocks(css)) {
      const tokens = collectDecls(decls)
      const names = Object.keys(tokens)
      if (!names.length) continue
      names.sort()
      const sorted = {}
      for (const name of names) sorted[name] = tokens[name]
      blocks.push({ file, selector: trail.join(' > '), tokens: sorted })
    }
  }

  // Stable block order: by file, then by selector.
  blocks.sort((a, b) => (a.file === b.file ? a.selector.localeCompare(b.selector) : a.file.localeCompare(b.file)))

  return {
    generatedBy: 'web/scripts/ds-tokens-export.mjs',
    note: 'Canonical design-token snapshot. Edit tokens in src/styles/*.css, then run `bun run ds:tokens`.',
    sources,
    blocks,
  }
}

export const serialize = (snapshot) => `${JSON.stringify(snapshot, null, 2)}\n`

export const readSnapshot = (cwd) => {
  const abs = path.join(cwd, SNAPSHOT_PATH)
  if (!fs.existsSync(abs)) return null
  return JSON.parse(fs.readFileSync(abs, 'utf8'))
}

/** Flatten to `selector :: token -> value` for diffing. */
const flatten = (snapshot) => {
  const flat = new Map()
  for (const block of snapshot?.blocks ?? []) {
    for (const [name, value] of Object.entries(block.tokens)) {
      flat.set(`${block.selector} :: ${name}`, value)
    }
  }
  return flat
}

export const diffSnapshots = (expected, actual) => {
  const want = flatten(expected)
  const have = flatten(actual)
  const added = []
  const removed = []
  const changed = []
  for (const [key, value] of want) {
    if (!have.has(key)) added.push(key)
    else if (have.get(key) !== value) changed.push({ key, before: value, after: have.get(key) })
  }
  for (const key of have.keys()) if (!want.has(key)) removed.push(key)
  return { added, removed, changed }
}
