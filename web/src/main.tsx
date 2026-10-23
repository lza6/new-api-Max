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
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'

import { installBuildMetadata } from '@/lib/build-metadata'
import { applyFaviconToDom } from '@/lib/dom-utils'
import '@/lib/dayjs'
import { initializeFrontendCache } from '@/lib/frontend-cache'
import { createAppQueryClient } from '@/lib/query-client'
import { readCachedStatus, statusQueryOptions } from '@/lib/status-query'

import { DirectionProvider } from './context/direction-provider'
import { ThemeProvider } from './context/theme-provider'
import i18n, { i18nReady } from './i18n/config'
import { routeLocaleBasepath } from './i18n/languages'
// Generated Routes
import { routeTree } from './routeTree.gen'

// Styles
import './styles/index.css'

// Ensure VChart theme is initialized before any chart mounts (prevents white default theme flash)
// VChart theme is driven by our ThemeProvider (html.light/html.dark) via per-chart `theme` prop.
initializeFrontendCache()
installBuildMetadata()

const queryClient = createAppQueryClient(() => {
  void router.navigate({ to: '/500' })
})

// Create a new router instance
// SEO 多语言：`/en/...`、`/fr/...` 等前缀由 TanStack Router 的 basepath 剥离。
// 默认语言 zh-CN 不带前缀（basepath undefined），现有 URL 不受影响。
const localeBasepath =
  typeof window === 'undefined'
    ? undefined
    : routeLocaleBasepath(window.location.pathname)
const router = createRouter({
  routeTree,
  ...(localeBasepath ? { basepath: localeBasepath } : {}),
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 30_000,
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// Render the app
const rootElement = document.querySelector<HTMLElement>('#root')
if (!rootElement) {
  throw new Error('Root element not found')
}
// Runtime SEO title. The brand name (from system config) and the translated
// tagline arrive asynchronously from two independent sources (the status query
// and the i18n bundle), in either order. So we keep the latest brand name in
// module scope and re-run `applyBrandingTitle` whenever EITHER resolves — the
// second event always produces the fully composed title.
let seoBrandName: string | null = null
function applyBrandingTitle() {
  if (typeof document === 'undefined') {return}
  const name = seoBrandName
  const tagline = i18n.t('AI API Gateway')
  const composed =
    name && tagline ? `${name} · ${tagline}` : name || tagline || ''
  if (!composed) {return}
  document.title = composed
  const metaTitle = document.querySelector(
    'meta[name="title"]'
  ) as HTMLMetaElement | null
  if (metaTitle) {metaTitle.setAttribute('content', composed)}
  for (const sel of ['meta[property="og:title"]', 'meta[name="twitter:title"]']) {
    const el = document.querySelector(sel) as HTMLMetaElement | null
    if (el) {el.setAttribute('content', composed)}
  }
}

// Set document.title and favicon from cached status, then refresh from network
;(function initSystemBranding() {
  try {
    if (typeof window === 'undefined' || typeof document === 'undefined') {return}
    const apply = (name: string) => {
      seoBrandName = name
      applyBrandingTitle()
    }
    // Hydrate relative canonical/og:url/og:image into absolute URLs — crawlers
    // and social scrapers require absolute values, but the built HTML ships
    // relative so the same artifact works on any host.
    const absolutize = (value: string) => {
      try {
        return new URL(value, window.location.origin).href
      } catch {
        return value
      }
    }
    const hydrateAbsoluteMeta = () => {
      for (const [sel, attr] of [
        ['link[rel="canonical"]', 'href'],
        ['meta[property="og:url"]', 'content'],
        ['meta[property="og:image"]', 'content'],
      ] as const) {
        const el = document.querySelector(sel)
        const current = el?.getAttribute(attr)
        if (el && current) {el.setAttribute(attr, absolutize(current))}
      }
    }
    hydrateAbsoluteMeta()
    // Cache-first
    const cached = readCachedStatus()
    if (cached?.system_name) {apply(cached.system_name as string)}
    if (cached?.logo) {applyFaviconToDom(cached.logo as string)}

    // Background refresh through the shared cache. This primes ['status']
    // before React mounts, so the root guard and every status consumer reuse
    // this one request instead of firing their own. `fetchStatus` owns the
    // localStorage write and the system-config store sync.
    queryClient
      .ensureQueryData(statusQueryOptions)
      .then((s) => {
        if (s?.system_name) {apply(s.system_name as string)}
        if (s?.logo) {applyFaviconToDom(s.logo as string)}
      })
      .catch(() => {
        /* empty */
      })
  } catch {
    /* empty */
  }
})()
// Re-run once the locale bundle resolves so the tagline suffix uses the
// translated string; if the brand name is not yet known this still stamps the
// tagline, and the status `.then` above will compose the final title later.
void i18nReady.then(() => {
  applyBrandingTitle()
})
if ('serviceWorker' in navigator && import.meta.env.PROD) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {})
  })
}

// #root 初始含 index.html 的加载指示（app-boot-loader）；React 首次渲染会替换它。
// 用替换而非 append：createRoot().render() 会接管 #root 的全部子节点。
const root = ReactDOM.createRoot(rootElement)
// 等语言包就绪再挂载：避免首屏（尤其 Setup 向导）用 fallback 渲染后闪切当前语言。
// 所有语言包（含 en）均为按需加载；加载失败时 i18nReady 也会 resolve，不永久阻塞首屏。
// 注：曾试过「最多等 300ms 再渲染」的降级方案，Playwright 实测无收益（瓶颈在
// JS 解析与后续路由 chunk 的串行，不在等待语言包本身），且会让慢网用户先看到
// 英文再切中文，已回退。
void i18nReady.then(() => {
  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <DirectionProvider>
            <RouterProvider router={router} />
          </DirectionProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </StrictMode>
  )
})
