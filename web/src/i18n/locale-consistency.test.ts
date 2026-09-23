import { describe, expect, test } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'

/*
 * 参考 imagefree-2ai 的 i18n 一致性测试做法（91 key 一致性测试锁定）：
 * 所有语言文件的 translation key 集合必须与 en 完全一致，防止新增 key 只加在
 * 个别语言导致中文/其它语言漏译；同时锁死"顶层命名空间 key = 0"（v1.3.3 曾把
 * 49 个顶层 key 并入 translation，禁止回退）。
 */
const LANGS = ['en', 'zh', 'zh-TW', 'fr', 'ja', 'ru', 'vi']
const localesDir = path.resolve(process.cwd(), 'src/i18n/locales')

function loadTranslationKeys(lang: string): string[] {
  const raw = fs.readFileSync(path.join(localesDir, `${lang}.json`), 'utf8')
  const json = JSON.parse(raw) as {
    translation: Record<string, unknown>
    [key: string]: unknown
  }
  return Object.keys(json.translation).sort()
}

describe('i18n locale consistency', () => {
  test('all locales define identical translation key sets (no missing / extra keys)', () => {
    const baseline = loadTranslationKeys('en')
    expect(baseline.length).toBeGreaterThan(0)
    for (const lang of LANGS) {
      const keys = loadTranslationKeys(lang)
      const missing = baseline.filter((k) => !keys.includes(k))
      const extra = keys.filter((k) => !baseline.includes(k))
      expect(missing, `${lang} missing keys`).toEqual([])
      expect(extra, `${lang} extra keys`).toEqual([])
    }
  })

  test('no top-level keys outside the translation namespace', () => {
    for (const lang of LANGS) {
      const raw = fs.readFileSync(path.join(localesDir, `${lang}.json`), 'utf8')
      const json = JSON.parse(raw) as Record<string, unknown>
      const topLevel = Object.keys(json).filter((k) => k !== 'translation')
      expect(topLevel, `${lang} top-level keys`).toEqual([])
    }
  })
})