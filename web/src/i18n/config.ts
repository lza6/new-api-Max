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
import LanguageDetector from 'i18next-browser-languagedetector'
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

const loadedLanguages = new Set<string>(['en'])
const loadingPromises = new Map<string, Promise<void>>()

/**
 * 确保指定语言包已加载并注册到 i18next。已加载则直接返回。
 * 切换语言前调用；失败时静默降级（保留 fallback en），不阻塞 UI。
 */
export async function ensureLanguageLoaded(code: string): Promise<void> {
  const lang = code || 'en'
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

export const resources = {
  en,
} as const

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    supportedLngs: ['en', 'zhCN', 'fr', 'ru', 'ja', 'vi', 'zhTW'],
    load: 'currentOnly',
    nsSeparator: false, // Allow literal colons in keys (e.g., URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: {
      escapeValue: false, // not needed for react as it escapes by default
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      // Browsers report `zh-CN`/`zh-TW`/`zh`; map them onto our `zhCN`/`zhTW`
      // codes (non-Chinese codes pass through for normal supportedLngs matching).
      convertDetectedLanguage,
    },
  })

// 初始化后，若检测到的语言不是 en，异步加载其语言包（不阻塞首屏）。
const detected = i18n.language
if (detected && detected !== 'en') {
  void ensureLanguageLoaded(detected)
}

export default i18n
