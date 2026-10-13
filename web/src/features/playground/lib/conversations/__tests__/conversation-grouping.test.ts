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
import { describe, expect, test } from 'vitest'

import {
  CONVERSATION_BUCKET_LABEL_KEY,
  CONVERSATION_BUCKET_ORDER,
  filterConversationsByTitle,
  getConversationBucket,
  groupConversationsByDate,
  startOfLocalDay,
} from '../conversation-grouping'

// 固定一个「今天」的参考时刻：2026-06-15 14:00 本地时间。
// 注入 now 而不是依赖真实当前时间，避免用例在跨零点、跨时区、CI 机器时钟下偶发失败。
const NOW = new Date(2026, 5, 15, 14, 0, 0, 0).getTime()
const DAY = 24 * 60 * 60 * 1000

function at(daysAgo: number, hour = 12): number {
  const d = new Date(NOW)
  d.setDate(d.getDate() - daysAgo)
  d.setHours(hour, 0, 0, 0)
  return d.getTime()
}

describe('startOfLocalDay', () => {
  test('zeroes the time part in local time', () => {
    const d = new Date(startOfLocalDay(NOW))
    expect(d.getHours()).toBe(0)
    expect(d.getMinutes()).toBe(0)
    expect(d.getSeconds()).toBe(0)
    expect(d.getMilliseconds()).toBe(0)
  })
})

describe('getConversationBucket', () => {
  test('classifies same-day timestamps as today, including earlier today', () => {
    expect(getConversationBucket(NOW, NOW)).toBe('today')
    // 今天凌晨也算今天（自然日切分，而不是「距今 24 小时内」）。
    expect(getConversationBucket(at(0, 0), NOW)).toBe('today')
    expect(getConversationBucket(at(0, 23), NOW)).toBe('today')
  })

  test('classifies the previous calendar day as yesterday', () => {
    expect(getConversationBucket(at(1, 23), NOW)).toBe('yesterday')
    expect(getConversationBucket(at(1, 0), NOW)).toBe('yesterday')
  })

  test('keeps 昨晚 23:59 与 今天 00:01 分属不同分组', () => {
    // 这是「按自然日而非 24 小时」的核心价值：两者实际只差 2 分钟，
    // 但用户认的是「昨天」和「今天」。
    const lastNight = new Date(2026, 5, 14, 23, 59, 0, 0).getTime()
    const thisMorning = new Date(2026, 5, 15, 0, 1, 0, 0).getTime()
    expect(getConversationBucket(lastNight, NOW)).toBe('yesterday')
    expect(getConversationBucket(thisMorning, NOW)).toBe('today')
  })

  test('classifies 2..7 days ago as the previous 7 days', () => {
    expect(getConversationBucket(at(2), NOW)).toBe('last7Days')
    expect(getConversationBucket(at(7), NOW)).toBe('last7Days')
  })

  test('classifies 8..30 days ago as the previous 30 days', () => {
    expect(getConversationBucket(at(8), NOW)).toBe('last30Days')
    expect(getConversationBucket(at(30), NOW)).toBe('last30Days')
  })

  test('classifies anything older as older', () => {
    expect(getConversationBucket(at(31), NOW)).toBe('older')
    expect(getConversationBucket(at(400), NOW)).toBe('older')
  })

  test('treats future timestamps as today instead of producing a negative bucket', () => {
    // 系统时钟漂移或用户手改时间会产生「未来」的 updatedAt；不能因此把它
    // 分到一个不存在的负偏移分组里（那样会话会从侧边栏消失）。
    expect(getConversationBucket(NOW + 3 * DAY, NOW)).toBe('today')
  })

  test('treats non-finite timestamps as older rather than throwing', () => {
    expect(getConversationBucket(Number.NaN, NOW)).toBe('older')
    expect(getConversationBucket(Number.POSITIVE_INFINITY, NOW)).toBe('older')
  })

  test('is stable regardless of the hour within a day for multi-day buckets', () => {
    // 3 天前的任意时刻都该落在同一分组。
    for (const hour of [0, 6, 12, 23]) {
      expect(getConversationBucket(at(3, hour), NOW)).toBe('last7Days')
    }
  })
})

describe('groupConversationsByDate', () => {
  const conv = (id: string, updatedAt: number) => ({
    id,
    title: `conv ${id}`,
    updatedAt,
  })

  test('groups in newest-first bucket order', () => {
    const groups = groupConversationsByDate(
      [
        conv('old', at(100)),
        conv('today', at(0, 9)),
        conv('week', at(3)),
        conv('yesterday', at(1, 20)),
        conv('month', at(20)),
      ],
      NOW
    )

    expect(groups.map((g) => g.bucket)).toEqual([
      'today',
      'yesterday',
      'last7Days',
      'last30Days',
      'older',
    ])
    expect(groups[0].conversations.map((c) => c.id)).toEqual(['today'])
    expect(groups[1].conversations.map((c) => c.id)).toEqual(['yesterday'])
  })

  test('sorts within a bucket by most recently updated first', () => {
    const groups = groupConversationsByDate(
      [conv('a', at(0, 8)), conv('b', at(0, 20)), conv('c', at(0, 12))],
      NOW
    )

    expect(groups).toHaveLength(1)
    expect(groups[0].conversations.map((c) => c.id)).toEqual(['b', 'c', 'a'])
  })

  test('omits empty buckets so no bare headers render', () => {
    const groups = groupConversationsByDate([conv('only', at(0, 10))], NOW)

    expect(groups).toHaveLength(1)
    expect(groups[0].bucket).toBe('today')
  })

  test('returns an empty array for no conversations', () => {
    expect(groupConversationsByDate([], NOW)).toEqual([])
  })

  test('does not mutate the input array', () => {
    const input = [conv('a', at(0, 8)), conv('b', at(0, 20))]
    const before = input.map((c) => c.id)

    groupConversationsByDate(input, NOW)

    expect(input.map((c) => c.id)).toEqual(before)
  })
})

describe('filterConversationsByTitle', () => {
  const list = [
    { id: '1', title: 'DeepSeek 低价可行性分析' },
    { id: '2', title: 'GPT-5 rollout plan' },
    { id: '3', title: '图片生成测试' },
  ]

  test('returns all conversations for an empty query', () => {
    // 搜索框清空必须恢复全部——返回空列表会让用户以为会话丢了。
    expect(filterConversationsByTitle(list, '')).toHaveLength(3)
    expect(filterConversationsByTitle(list, '   ')).toHaveLength(3)
  })

  test('matches case-insensitively on a substring', () => {
    expect(filterConversationsByTitle(list, 'gpt').map((c) => c.id)).toEqual(['2'])
    expect(filterConversationsByTitle(list, 'GPT').map((c) => c.id)).toEqual(['2'])
  })

  test('matches CJK substrings', () => {
    expect(filterConversationsByTitle(list, '图片').map((c) => c.id)).toEqual(['3'])
    expect(filterConversationsByTitle(list, '可行').map((c) => c.id)).toEqual(['1'])
  })

  test('returns an empty list when nothing matches', () => {
    expect(filterConversationsByTitle(list, 'zzzz')).toEqual([])
  })

  test('ignores surrounding whitespace in the query', () => {
    expect(filterConversationsByTitle(list, '  图片  ').map((c) => c.id)).toEqual(['3'])
  })
})

describe('CONVERSATION_BUCKET_ORDER / LABEL_KEY', () => {
  test('covers every bucket exactly once, newest first', () => {
    expect(CONVERSATION_BUCKET_ORDER).toEqual([
      'today',
      'yesterday',
      'last7Days',
      'last30Days',
      'older',
    ])
    expect(new Set(CONVERSATION_BUCKET_ORDER).size).toBe(
      CONVERSATION_BUCKET_ORDER.length
    )
  })

  test('provides a non-empty label key for every bucket', () => {
    // 漏一个会让该分组渲染出空标题（或 i18n 回退成键名）。
    for (const bucket of CONVERSATION_BUCKET_ORDER) {
      const key = CONVERSATION_BUCKET_LABEL_KEY[bucket]
      expect(key, bucket).toBeTruthy()
      expect(key.trim().length, bucket).toBeGreaterThan(0)
    }
  })
})
