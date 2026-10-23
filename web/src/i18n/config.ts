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
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import {
  convertDetectedLanguage,
  languageFromPathPrefix,
  pathWithLanguage,
  type InterfaceLanguageCode,
} from './languages'
// [性能] 所有语言包（含 fallback en）统一按需动态 import。en 不再静态打包：
// 其键就是英文源串，某语言缺键时会自然回退为英文键文本，无需把整份 en 压进
// 首屏 bundle。首屏只下载当前语言（实测各语言键数与 en 完全一致，6842/6842）。
// 切换语言时用 ensureLanguageLoaded() 先加载对应语言包再 changeLanguage。

/** 语言包懒加载器：每个语言一个动态 import（被拆成独立 chunk）。 */
const localeLoaders: Record<string, () => Promise<{ default: unknown }>> = {
  en: () => import(/* webpackChunkName: 'i18n-full-en' */ './locales/en.json'),
  zhCN: () => import(/* webpackChunkName: 'i18n-full-zhCN' */ './locales/zh.json'),
  zhTW: () => import(/* webpackChunkName: 'i18n-full-zhTW' */ './locales/zh-TW.json'),
  fr: () => import(/* webpackChunkName: 'i18n-full-fr' */ './locales/fr.json'),
  ru: () => import(/* webpackChunkName: 'i18n-full-ru' */ './locales/ru.json'),
  ja: () => import(/* webpackChunkName: 'i18n-full-ja' */ './locales/ja.json'),
  vi: () => import(/* webpackChunkName: 'i18n-full-vi' */ './locales/vi.json'),
}

/**
 * 首屏子集：只含壳层与公开页/登录页会渲染到的 key（`scripts/scan-i18n-common.mjs`
 * 生成，约 300 条 / 7KB gzip，而整包 175KB gzip）。
 *
 * `main.tsx` 的 `i18nReady` 只等这一份，所以首屏不再被整包语言包串行阻塞；
 * 整包在首屏渲染后后台补齐并触发一次重渲染。子集漏掉的 key 会短暂显示英文原键，
 * 因此 `web/e2e-local/i18n-no-flash.cjs` 会用真实浏览器逐页检测「渲染文本等于
 * 英文 key」的情况，漏一条就补进 `common-keys.json`。
 */
const commonLocaleLoaders: Record<string, () => Promise<{ default: unknown }>> =
  {
    en: () => import(/* webpackChunkName: 'i18n-common-en' */ './locales/en.common.json'),
    zhCN: () => import(/* webpackChunkName: 'i18n-common-zhCN' */ './locales/zh.common.json'),
    zhTW: () => import(/* webpackChunkName: 'i18n-common-zhTW' */ './locales/zh-TW.common.json'),
    fr: () => import(/* webpackChunkName: 'i18n-common-fr' */ './locales/fr.common.json'),
    ru: () => import(/* webpackChunkName: 'i18n-common-ru' */ './locales/ru.common.json'),
    ja: () => import(/* webpackChunkName: 'i18n-common-ja' */ './locales/ja.common.json'),
    vi: () => import(/* webpackChunkName: 'i18n-common-vi' */ './locales/vi.common.json'),
  }

const SUPPORTED = ['en', 'zhCN', 'zhTW', 'fr', 'ru', 'ja', 'vi']
const LANGUAGE_STORAGE_KEY = 'i18nextLng'
/** 默认语言：简体中文（新访客）。 */
const DEFAULT_LANGUAGE = 'zhCN'

const loadedCommon = new Set<string>()
const commonLoadPromises = new Map<string, Promise<void>>()
const loadedFull = new Set<string>()
const fullLoadPromises = new Map<string, Promise<void>>()

type LocaleModule = { default: unknown }

/** 语言 JSON 形如 { translation: {...} }；剥掉外层后交给 addResourceBundle。 */
function bundlePayload(mod: LocaleModule): Record<string, unknown> {
  const bundle = mod.default as { translation?: Record<string, unknown> }
  return bundle?.translation ?? (mod.default as Record<string, unknown>)
}

function loadBundle(
  lang: string,
  loader: (() => Promise<LocaleModule>) | undefined,
  loaded: Set<string>,
  inFlight: Map<string, Promise<void>>
): Promise<void> {
  if (!loader || loaded.has(lang)) {return Promise.resolve()}
  const existing = inFlight.get(lang)
  if (existing) {return existing}
  const promise = loader()
    .then((mod) => {
      i18n.addResourceBundle(lang, 'translation', bundlePayload(mod), true, true)
      loaded.add(lang)
    })
    .catch(() => {
      // 加载失败：保留已注册的 fallback，不阻塞交互。
    })
    .finally(() => {
      inFlight.delete(lang)
    })
  inFlight.set(lang, promise)
  return promise
}

/** 首屏子集（阻塞首帧）。 */
function ensureCommonLanguageLoaded(code: string): Promise<void> {
  const lang = code || DEFAULT_LANGUAGE
  return loadBundle(lang, commonLocaleLoaders[lang], loadedCommon, commonLoadPromises)
}

/** 整包语言包（后台补齐；显式切换语言时也用它）。 */
function ensureFullLanguageLoaded(code: string): Promise<void> {
  const lang = code || DEFAULT_LANGUAGE
  return loadBundle(lang, localeLoaders[lang], loadedFull, fullLoadPromises)
}

/**
 * 确保指定语言**完整**可用。切换语言前调用；失败时静默降级（保留 fallback en），
 * 不阻塞 UI。
 */
async function ensureLanguageLoaded(code: string): Promise<void> {
  const lang = code || DEFAULT_LANGUAGE
  await ensureCommonLanguageLoaded(lang)
  await ensureFullLanguageLoaded(lang)
}

/** 读取用户显式选择的语言（localStorage）；无则 null。 */
function readStoredLanguage(): string | null {
  if (typeof window === 'undefined') {
    return null
  }
  try {
    const raw = window.localStorage.getItem(LANGUAGE_STORAGE_KEY)
    if (!raw) {
      return null
    }
    // convertDetectedLanguage 归一化（如 zh-CN -> zhCN）；非中文原样返回。
    const normalized = convertDetectedLanguage(raw)
    return SUPPORTED.includes(normalized) ? normalized : null
  } catch {
    return null
  }
}

/** 读取 URL 路径前缀里的语言（`/en/...`、`/zh-TW/...`）；无前缀返回 null。 */
function readPathLanguage(): string | null {
  if (typeof window === 'undefined' || !window.location) {
    return null
  }
  return languageFromPathPrefix(window.location.pathname)
}

/** 写入用户显式选择的语言到 localStorage。 */
function persistLanguage(code: string): void {
  if (typeof window === 'undefined') {
    return
  }
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, code)
  } catch {
    // 忽略存储失败（隐私模式等）。
  }
}

/** 内部语言码 → BCP-47（Intl/document.lang 用）。 */
const BCP47_BY_CODE: Record<string, string> = {
  zhCN: 'zh-CN',
  zhTW: 'zh-TW',
}

/** 把 <html lang> 同步为当前语言（BCP-47），利于 a11y/SEO 与浏览器翻译提示。 */
function syncHtmlLang(code: string): void {
  if (typeof document === 'undefined') {
    return
  }
  document.documentElement.lang = BCP47_BY_CODE[code] ?? (code || 'en')
}

/**
 * 切换语言：按需加载语言包 → 持久化用户选择 → 应用。所有切换入口都应调用它
 * （而不是裸 changeLanguage），以保证语言包已就绪且用户选择被记住。
 */
export async function setLanguage(code: string): Promise<void> {
  await ensureLanguageLoaded(code)
  persistLanguage(code)
  await i18n.changeLanguage(code)
  syncHtmlLang(code)
}

export const resources = {} as const

/** 服务端注入内容的公开页：这些页面的 canonical/hreflang 必须与 URL 语言一致。 */
const PUBLIC_SEO_PATHS = new Set([
  '/',
  '/pricing',
  '/about',
  '/rankings',
  '/model-test',
  '/tool-setup',
  '/status',
  '/user-agreement',
  '/privacy-policy',
])

/**
 * 存量用户偏好与无前缀公开页冲突时，先跳到对应语言前缀再渲染。
 *
 * 例：用户存过 en，直接打开外链 `/pricing`（服务端 canonical=/pricing、hreflang
 * zh-CN），若首屏渲染英文就会与 canonical 矛盾。跳转到 `/en/pricing` 后，
 * URL、内容、canonical、hreflang 四者一致。
 */
function redirectToPreferredLocale(): void {
  if (typeof window === 'undefined' || !window.location) {
    return
  }
  const pathname = window.location.pathname
  if (languageFromPathPrefix(pathname)) {
    return
  }
  const normalized =
    pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  if (!PUBLIC_SEO_PATHS.has(normalized)) {
    return
  }
  const stored = readStoredLanguage()
  if (!stored || stored === DEFAULT_LANGUAGE) {
    return
  }
  const target = pathWithLanguage(pathname, stored as InterfaceLanguageCode)
  window.location.replace(
    target + window.location.search + window.location.hash
  )
}

redirectToPreferredLocale()

// 初始语言优先级：URL 语言前缀 → 显式选择（localStorage）→ 默认简体中文。
// URL 前缀必须优先：`/en/pricing` 是分享出去的英文页，不能因为访客本地存过中文
// 就渲染成中文（那会让页面语言与 canonical/hreflang 不一致）。
const initialLanguage =
  readPathLanguage() ?? readStoredLanguage() ?? DEFAULT_LANGUAGE

i18n.use(initReactI18next).init({
  resources,
  lng: initialLanguage,
  fallbackLng: 'en',
  supportedLngs: SUPPORTED,
  load: 'currentOnly',
  nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
  debug: import.meta.env.DEV,
  // 记录「当前语言缺哪些 key」：首屏只加载 common 子集时，任何缺失的 key 都会
  // 回退成英文原键（= 用户看到的「英文闪切」）。`e2e-local/i18n-common-audit.cjs`
  // 会读这个数组，把漏掉的 key 并回 `common-keys.json`。
  saveMissing: true,
  missingKeyHandler: (_lngs: readonly string[], _ns: string, key: string) => {
    if (typeof window === 'undefined') {return}
    const w = window as Window & { __i18nMissing?: string[] }
    w.__i18nMissing ??= []
    if (!w.__i18nMissing.includes(key)) {w.__i18nMissing.push(key)}
  },
  interpolation: {
    escapeValue: false, // not needed for react as it escapes by default
  },
})

// 初始语言语言包加载完成后必须**无条件 changeLanguage**：i18n.language 初始就是
// initialLanguage（如 zhCN），但此时资源尚未加载，i18next 的 resolvedLanguage
// 会降级到 fallback；addResourceBundle 后需 changeLanguage 触发重新解析，才会
// 真正切换到该语言。仅当 language !== initialLanguage 才切是错的。
//
// i18nReady：语言就绪 promise。main.tsx 会 await 它再 render，确保首屏（含
// Setup 向导页）用正确语言渲染，而不是先用 fallback 渲染再闪切。
// 首屏只等「首屏子集」（~7KB gzip），整包在渲染后台补齐后再补一次文案。
// 不采用「先按英文渲染再切中文」：那会闪英文，业主明确不接受。
export const i18nReady: Promise<void> = (async () => {
  await ensureCommonLanguageLoaded(initialLanguage)
  await i18n.changeLanguage(initialLanguage)
  void ensureFullLanguageLoaded(initialLanguage).then(() => {
    // addResourceBundle 不会自动触发重渲染，主动通知一次。
    i18n.emit('languageChanged', i18n.language)
  })
})()

// 首屏语言确定后同步 <html lang>（i18nReady 完成后即可；同步设置一次）。
void i18nReady.then(() => syncHtmlLang(initialLanguage))

export default i18n


