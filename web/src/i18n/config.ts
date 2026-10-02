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

import { convertDetectedLanguage } from './languages'
// [性能] 只静态打包 en（fallback，任何语言缺失键时都要用）。其余 6 个语言包
// 合计约 3.3MB，改为按需动态 import —— 首屏只加载当前语言，把入口 bundle 从
// ~3.7MB 降到 ~0.5MB（跨境弱网下这是能否打开的关键）。切换语言时用
// ensureLanguageLoaded() 先加载对应语言包再 changeLanguage。
import en from './locales/en.json'

/** 语言包懒加载器：每个语言一个动态 import（被拆成独立 chunk）。 */
const localeLoaders: Record<string, () => Promise<{ default: unknown }>> = {
  en: async () => ({ default: en }),
  zhCN: () => import('./locales/zh.json'),
  zhTW: () => import('./locales/zh-TW.json'),
  fr: () => import('./locales/fr.json'),
  ru: () => import('./locales/ru.json'),
  ja: () => import('./locales/ja.json'),
  vi: () => import('./locales/vi.json'),
}

const SUPPORTED = ['en', 'zhCN', 'zhTW', 'fr', 'ru', 'ja', 'vi']
const LANGUAGE_STORAGE_KEY = 'i18nextLng'
/** 默认语言：简体中文（新访客）。 */
const DEFAULT_LANGUAGE = 'zhCN'

const loadedLanguages = new Set<string>(['en'])
const loadingPromises = new Map<string, Promise<void>>()

/**
 * 确保指定语言包已加载并注册到 i18next。已加载则直接返回。
 * 切换语言前调用；失败时静默降级（保留 fallback en），不阻塞 UI。
 */
export async function ensureLanguageLoaded(code: string): Promise<void> {
  const lang = code || DEFAULT_LANGUAGE
  if (loadedLanguages.has(lang)) {
    return
  }
  const existing = loadingPromises.get(lang)
  if (existing) {
    return existing
  }
  const loader = localeLoaders[lang]
  if (!loader) {
    return
  }
  const promise = loader()
    .then((mod) => {
      i18n.addResourceBundle(lang, 'translation', mod.default, true, true)
      loadedLanguages.add(lang)
    })
    .catch(() => {
      // 加载失败：保留 fallback（en），不抛出——避免语言包网络问题阻塞交互。
    })
    .finally(() => {
      loadingPromises.delete(lang)
    })
  loadingPromises.set(lang, promise)
  return promise
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

/** 写入用户显式选择的语言到 localStorage。 */
export function persistLanguage(code: string): void {
  if (typeof window === 'undefined') {
    return
  }
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, code)
  } catch {
    // 忽略存储失败（隐私模式等）。
  }
}

/**
 * 切换语言：按需加载语言包 → 持久化用户选择 → 应用。所有切换入口都应调用它
 * （而不是裸 changeLanguage），以保证语言包已就绪且用户选择被记住。
 */
export async function setLanguage(code: string): Promise<void> {
  await ensureLanguageLoaded(code)
  persistLanguage(code)
  await i18n.changeLanguage(code)
}

export const resources = {
  en,
} as const

// 初始语言：显式选择优先，否则默认简体中文。
// 不使用 LanguageDetector 的浏览器语言检测——避免非中文浏览器被带到英文；
// 显式选择由 persistLanguage 写入 localStorage，此处直接读取决定 lng。
const initialLanguage = readStoredLanguage() ?? DEFAULT_LANGUAGE

i18n.use(initReactI18next).init({
  resources,
  lng: initialLanguage,
  fallbackLng: 'en',
  supportedLngs: SUPPORTED,
  load: 'currentOnly',
  nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
  debug: import.meta.env.DEV,
  interpolation: {
    escapeValue: false, // not needed for react as it escapes by default
  },
})

// 初始语言非 en 时异步加载其语言包（不阻塞首屏）。
// [关键] 加载完成后必须**无条件 changeLanguage**：i18n.language 初始就是
// initialLanguage（如 zhCN），但此时资源尚未加载，i18next 的 resolvedLanguage
// 会降级到 fallback en；addResourceBundle 后需 changeLanguage 触发重新解析，
// 才会真正切换到该语言。仅当 language !== initialLanguage 才切是错的。
if (initialLanguage !== 'en') {
  void ensureLanguageLoaded(initialLanguage).then(() => i18n.changeLanguage(initialLanguage))
}

export default i18n

