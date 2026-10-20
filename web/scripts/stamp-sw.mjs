/**
 * Stamp `dist/sw.js` with a cache version derived from the emitted assets.
 *
 * Why: `public/sw.js` is copied verbatim to `dist/`, so a hard-coded VERSION
 * never changes between releases. The activate handler only deletes caches whose
 * key differs from VERSION, so old entries accumulated forever (and `cache.put`
 * kept growing until the origin quota kicked in).
 *
 * The version is `package.version` + a short hash of the hashed asset names, so
 * it changes exactly when the build output changes — not on every rebuild.
 *
 * Usage: `node scripts/stamp-sw.mjs` (runs as part of `bun run build`).
 */
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'

const root = process.cwd()
const dist = join(root, 'dist')
const swPath = join(dist, 'sw.js')

if (!existsSync(swPath)) {
  console.error('[stamp-sw] dist/sw.js not found — run the build first')
  process.exit(1)
}

/** Collect every emitted static file name (hashes included) deterministically. */
function collect(dir, out = []) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) {
      collect(full, out)
    } else {
      out.push(relative(dist, full).replaceAll('\\', '/'))
    }
  }
  return out
}

const assetRoot = join(dist, 'static')
const assets = existsSync(assetRoot) ? collect(assetRoot).sort() : []
const assetHash = createHash('sha256').update(assets.join('|')).digest('hex').slice(0, 8)

const pkg = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'))
const version = `${pkg.version}-${assetHash}`

const source = readFileSync(swPath, 'utf8')
const stamped = source.replace(/const VERSION = '[^']*'/, `const VERSION = '${version}'`)

if (stamped === source) {
  console.error('[stamp-sw] VERSION placeholder not found in dist/sw.js')
  process.exit(1)
}

writeFileSync(swPath, stamped)
console.log(`[stamp-sw] VERSION = ${version} (${assets.length} assets)`)
