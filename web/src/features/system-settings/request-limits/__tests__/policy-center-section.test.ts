import { describe, expect, test } from 'vitest'

import {
  parseModelLimits,
  parseModelList,
  serializeModelLimits,
  toModelLimitsText,
  toModelListText,
} from '../policy-center-section'

describe('parseModelLimits', () => {
  test('parses model=quota lines and ignores comments and blanks', () => {
    const result = parseModelLimits(
      '# per-model budget\ngpt-4o=50000\n\nclaude-3=120000'
    )
    expect(result).toEqual({
      ok: true,
      value: { 'gpt-4o': 50000, 'claude-3': 120000 },
    })
  })

  test('empty text means no per-model limit', () => {
    expect(parseModelLimits('')).toEqual({ ok: true, value: {} })
  })

  test('rejects a line without a separator', () => {
    expect(parseModelLimits('gpt-4o 50000')).toEqual({
      ok: false,
      line: 'gpt-4o 50000',
    })
  })

  test('rejects non-numeric and negative limits', () => {
    expect(parseModelLimits('gpt-4o=abc')).toEqual({ ok: false, line: 'gpt-4o=abc' })
    expect(parseModelLimits('gpt-4o=-5')).toEqual({ ok: false, line: 'gpt-4o=-5' })
  })

  test('keeps a model name that itself contains a separator', () => {
    expect(parseModelLimits('vendor:model=gpt=100')).toEqual({
      ok: true,
      value: { 'vendor:model=gpt': 100 },
    })
  })
})

describe('serializeModelLimits', () => {
  test('sorts keys so identical config yields identical text', () => {
    expect(serializeModelLimits({ b: 2, a: 1 })).toBe('a=1\nb=2')
  })
})

describe('parseModelList', () => {
  test('trims, drops blanks and comments', () => {
    expect(parseModelList('gpt-4o\n\n# note\n claude-3 \n')).toEqual([
      'gpt-4o',
      'claude-3',
    ])
  })
})

describe('toModelLimitsText / toModelListText', () => {
  test('accepts a JSON string from the API', () => {
    expect(toModelLimitsText('{"gpt-4o":1000}')).toBe('gpt-4o=1000')
    expect(toModelListText('["gpt-4o","claude-3"]')).toBe('gpt-4o\nclaude-3')
  })

  test('accepts already-parsed values', () => {
    expect(toModelLimitsText({ 'gpt-4o': 1000 })).toBe('gpt-4o=1000')
    expect(toModelListText(['gpt-4o'])).toBe('gpt-4o')
  })

  test('returns empty text for malformed input instead of throwing', () => {
    expect(toModelLimitsText('not json')).toBe('')
    expect(toModelLimitsText(null)).toBe('')
    expect(toModelListText('{broken')).toBe('')
    expect(toModelListText(42)).toBe('')
  })

  test('drops non-numeric limits', () => {
    expect(toModelLimitsText({ 'gpt-4o': 'x', 'claude-3': 5 })).toBe('claude-3=5')
  })
})
