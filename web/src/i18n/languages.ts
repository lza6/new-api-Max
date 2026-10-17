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
export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zhCN', label: '简体中文' },
  { code: 'en', label: 'English' },
  { code: 'fr', label: 'Français' },
  { code: 'ru', label: 'Русский' },
  { code: 'ja', label: '日本語' },
  { code: 'vi', label: 'Tiếng Việt' },
  { code: 'zhTW', label: '繁體中文' },
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

/**
 * URL 路径前缀与界面语言码的映射（SEO hreflang 用）。
 *
 * 默认语言简体中文**不带前缀**（`/pricing` 就是 zh-CN 的 canonical）；
 * 其它语言使用 `/en/pricing`、`/zh-TW/pricing` 这类前缀。
 * 服务端 `controller/spa_seo.go` 维护同一份映射，改动必须两边同步。
 */
const ROUTE_PREFIX_TO_CODE: Record<string, InterfaceLanguageCode> = {
  en: 'en',
  fr: 'fr',
  ru: 'ru',
  ja: 'ja',
  vi: 'vi',
  'zh-tw': 'zhTW',
}

const CODE_TO_ROUTE_PREFIX: Record<InterfaceLanguageCode, string> = {
  zhCN: '',
  en: '/en',
  fr: '/fr',
  ru: '/ru',
  ja: '/ja',
  vi: '/vi',
  zhTW: '/zh-TW',
}

/** 从 URL 第一段解析语言码；不是受支持的语言前缀时返回 null。 */
export function languageFromPathPrefix(
  pathname: string
): InterfaceLanguageCode | null {
  const first = pathname.split('/').find(Boolean)?.toLowerCase()
  if (!first) {return null}
  return ROUTE_PREFIX_TO_CODE[first] ?? null
}

/** TanStack Router 的 basepath（默认语言返回 undefined，表示无前缀）。 */
export function routeLocaleBasepath(pathname: string): string | undefined {
  const code = languageFromPathPrefix(pathname)
  if (!code || code === 'zhCN') {return undefined}
  return CODE_TO_ROUTE_PREFIX[code]
}

/** 去掉 URL 中已有的语言前缀，返回站内路径。 */
export function stripRouteLocalePrefix(pathname: string): string {
  const code = languageFromPathPrefix(pathname)
  if (!code) {return pathname || '/'}
  const segments = pathname.split('/').filter(Boolean)
  const rest = `/${segments.slice(1).join('/')}`
  return rest === '/' ? '/' : rest.replace(/\/$/, '')
}

/** 给站内路径加上目标语言前缀（默认语言 zhCN 不加）。 */
export function pathWithLanguage(
  pathname: string,
  code: InterfaceLanguageCode
): string {
  const stripped = stripRouteLocalePrefix(pathname)
  const prefix = CODE_TO_ROUTE_PREFIX[code]
  if (!prefix) {return stripped}
  return stripped === '/' ? prefix : prefix + stripped
}

export function normalizeInterfaceLanguage(value?: string | null): string {
  if (!value) {return 'en'}

  let normalized = value.trim().replaceAll('_', '-').toLowerCase()
  if (
    value === 'zh-TW' ||
    value === 'zh-HK' ||
    value === 'zh-MO' ||
    value === 'zhTW'
  ) {
    normalized = 'zhTW'
  }
  if (value === 'zh-CN' || value === 'zh-Hans' || value === 'zhCN') {
    normalized = 'zhCN'
  }

  return INTERFACE_LANGUAGE_OPTIONS.some((lang) => lang.code === normalized)
    ? normalized
    : 'en'
}

/**
 * Map a browser-detected locale onto the interface language codes this project
 * uses with i18next (`zhCN` / `zhTW`).
 *
 * Browsers report standard BCP-47 tags (`zh-CN`, `zh-TW`, `zh-Hant`, `zh`, ...),
 * but `supportedLngs`/resources use the non-standard camelCase codes, so without
 * this mapping a Chinese browser would never match and fall back to English.
 * Non-Chinese codes are returned unchanged so i18next's own `supportedLngs`
 * matching still applies (e.g. `fr-FR` -> `fr`, `ja` -> `ja`).
 */
export function convertDetectedLanguage(value: string): string {
  const lower = value.trim().replaceAll('_', '-').toLowerCase()
  if (!lower.startsWith('zh')) {return value}
  if (
    lower === 'zh-tw' ||
    lower === 'zh-hk' ||
    lower === 'zh-mo' ||
    lower.startsWith('zh-hant')
  ) {
    return 'zhTW'
  }
  return 'zhCN'
}

/**
 * Convert an interface language code (the values i18next uses, such as `zhCN` /
 * `zhTW`) into a valid BCP-47 locale tag that the `Intl.*` APIs accept.
 *
 * `new Intl.NumberFormat('zhCN')` throws `RangeError: Invalid language tag`, so
 * any locale derived from `i18n.language` / `i18n.resolvedLanguage` MUST be run
 * through this before it reaches an `Intl` constructor. Unknown values fall back
 * to `undefined`, which makes `Intl` use the runtime default locale.
 */
export function toIntlLocale(value?: string | null): string | undefined {
  if (!value) {return undefined}
  switch (value) {
    case 'zhCN':
      return 'zh-CN'
    case 'zhTW':
      return 'zh-TW'
    default:
      break
  }
  try {
    return Intl.getCanonicalLocales(value)[0]
  } catch {
    return undefined
  }
}
