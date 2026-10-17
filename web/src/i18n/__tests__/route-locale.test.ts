import { describe, expect, it } from 'vitest'

import {
  languageFromPathPrefix,
  pathWithLanguage,
  routeLocaleBasepath,
  stripRouteLocalePrefix,
} from '../languages'

describe('route locale prefix', () => {
  it('parses supported locale prefixes case-insensitively', () => {
    expect(languageFromPathPrefix('/en/pricing')).toBe('en')
    expect(languageFromPathPrefix('/zh-TW/pricing')).toBe('zhTW')
    expect(languageFromPathPrefix('/ZH-tw')).toBe('zhTW')
    expect(languageFromPathPrefix('/fr')).toBe('fr')
  })

  it('treats unprefixed and ordinary routes as the default language', () => {
    expect(languageFromPathPrefix('/pricing')).toBeNull()
    expect(languageFromPathPrefix('/')).toBeNull()
    // 路由本身叫 /model-test，不能被误认成语言前缀。
    expect(languageFromPathPrefix('/model-test')).toBeNull()
  })

  it('returns a router basepath only for non-default languages', () => {
    expect(routeLocaleBasepath('/en/pricing')).toBe('/en')
    expect(routeLocaleBasepath('/zh-TW/pricing')).toBe('/zh-TW')
    expect(routeLocaleBasepath('/pricing')).toBeUndefined()
  })

  it('strips an existing prefix without leaving a trailing slash', () => {
    expect(stripRouteLocalePrefix('/en/pricing')).toBe('/pricing')
    expect(stripRouteLocalePrefix('/en')).toBe('/')
    expect(stripRouteLocalePrefix('/pricing')).toBe('/pricing')
  })

  it('rewrites a path to the target language prefix', () => {
    expect(pathWithLanguage('/pricing', 'en')).toBe('/en/pricing')
    expect(pathWithLanguage('/en/pricing', 'zhCN')).toBe('/pricing')
    expect(pathWithLanguage('/en/pricing', 'fr')).toBe('/fr/pricing')
    expect(pathWithLanguage('/', 'zhTW')).toBe('/zh-TW')
    expect(pathWithLanguage('/zh-TW', 'zhCN')).toBe('/')
  })
})
