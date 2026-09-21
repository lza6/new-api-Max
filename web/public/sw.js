/* Service Worker: cache hashed static assets only; never cache API/auth or HTML navigation. */
const VERSION = 'v1.2.33.1'

self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      await self.clients.claim()
      const keys = await caches.keys()
      await Promise.all(keys.filter((k) => k !== VERSION).map((k) => caches.delete(k)))
    })()
  )
})

self.addEventListener('fetch', (event) => {
  const { request } = event
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  if (request.mode === 'navigate') return
  const p = url.pathname
  if (p.startsWith('/api/') || p.startsWith('/v1/') || p.startsWith('/oauth/') || p.startsWith('/pg/') || p.startsWith('/mj/')) return
  if (!/\\.(js|css|woff2?|png|svg|ico|webp|jpg|jpeg)$/.test(p)) return

  event.respondWith(
    (async () => {
      const cache = await caches.open(VERSION)
      const hit = await cache.match(request)
      if (hit) return hit
      try {
        const res = await fetch(request)
        const copy = res.clone()
        await cache.put(request, copy)
        return res
      } catch {
        return hit || Response.error()
      }
    })()
  )
})
