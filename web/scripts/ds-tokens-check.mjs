/*
Design-token drift gate: fail when src/styles/*.css no longer matches the
committed snapshot.

Run from web/: bun run ds:tokens:check   (exits 1 on drift — usable in CI)

Fix by either restoring the CSS, or — when the change is intended — running
`bun run ds:tokens` and committing the snapshot diff.
*/
import { SNAPSHOT_PATH, collectTokens, diffSnapshots, readSnapshot } from './ds-tokens-lib.mjs'

const cwd = process.cwd()
const committed = readSnapshot(cwd)

if (!committed) {
  console.error(`ds-tokens: snapshot missing (${SNAPSHOT_PATH}). Run: bun run ds:tokens`)
  process.exit(1)
}

const live = collectTokens(cwd)
const { added, removed, changed } = diffSnapshots(committed, live)
const total = added.length + removed.length + changed.length

// A source file hash change with identical tokens is not drift (e.g. comment or
// layout edit in the same stylesheet). Report it, but never fail on it.
const hashDrift = (committed.sources ?? []).filter((source) => {
  const current = live.sources.find((s) => s.file === source.file)
  return current && current.sha256_16 !== source.sha256_16
})

if (!total) {
  const tokenCount = live.blocks.reduce((sum, b) => sum + Object.keys(b.tokens).length, 0)
  console.log(`ds-tokens: OK (${tokenCount} tokens, source hash ${hashDrift.length ? 'changed' : 'unchanged'})`)
  if (hashDrift.length) {
    for (const source of hashDrift) {
      console.log(`  note: ${source.file} changed outside tokens — snapshot value still matches`)
    }
  }
  process.exit(0)
}

console.error(`ds-tokens: DRIFT (${total}) — styles and snapshot disagree`)
for (const key of added) console.error(`  + ${key}`)
for (const key of removed) console.error(`  - ${key}`)
for (const item of changed) {
  console.error(`  ~ ${item.key}`)
  console.error(`      snapshot: ${item.before}`)
  console.error(`      css     : ${item.after}`)
}
console.error('\nIf the change is intended: bun run ds:tokens && commit the snapshot diff.')
process.exit(1)
