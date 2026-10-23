/*
Regenerate the canonical design-token snapshot (.ai/ds/ds-tokens.json).

Run from web/: bun run ds:tokens
Commit the resulting diff together with any change to src/styles/*.css — a token
change without a snapshot update is exactly the drift ds-tokens-check.mjs rejects.
*/
import fs from 'node:fs'
import path from 'node:path'

import { SNAPSHOT_PATH, collectTokens, serialize } from './ds-tokens-lib.mjs'

const cwd = process.cwd()
const snapshot = collectTokens(cwd)
const abs = path.join(cwd, SNAPSHOT_PATH)

if (!snapshot.blocks.length) {
  console.error('ds-tokens: no custom properties found — are src/styles/*.css still there?')
  process.exit(1)
}

const next = serialize(snapshot)
const previous = fs.existsSync(abs) ? fs.readFileSync(abs, 'utf8') : null

if (previous === next) {
  console.log(`ds-tokens: no change (${abs.replace(cwd + path.sep, '')})`)
  process.exit(0)
}

fs.mkdirSync(path.dirname(abs), { recursive: true })
fs.writeFileSync(abs, next)
const tokenCount = snapshot.blocks.reduce((sum, b) => sum + Object.keys(b.tokens).length, 0)
console.log(
  `ds-tokens: wrote ${abs.replace(cwd + path.sep, '')} — ${snapshot.blocks.length} blocks, ${tokenCount} tokens`
)
