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
// 会话列表按时间分组（纯函数，无 React/LocalStorage 依赖，便于单测）。
//
// 分组边界与参考项目（`自营的SaaS服务平台` 的 agent 会话列表）一致：
// **今天 / 昨天 / 7 天内 / 30 天内 / 更早**。
//
// 参考项目只有 今天/7天内/30天内/更早 四档（没有「昨天」）；这里**刻意加了「昨天」**：
// 用户回看「昨天聊的那个」是最常见的检索动作，把它并进「7 天内」会让最常见的
// 动作付出最多的扫描成本。
//
// 边界全部基于**自然日**而非「距今 N×24 小时」：凌晨 2 点打开的会话，用户认的是
// 「昨天」，而 26 小时前算出来也是「昨天」但 22 小时前的会被算成「今天」——
// 后者与直觉不符。因此按本地日期的零点切分。

/** 会话所属的时间分组。`older` 是兜底。 */
export type ConversationDateBucket =
  | 'today'
  | 'yesterday'
  | 'last7Days'
  | 'last30Days'
  | 'older'

/** 分组展示顺序（新 → 旧），也是渲染顺序。 */
export const CONVERSATION_BUCKET_ORDER: ConversationDateBucket[] = [
  'today',
  'yesterday',
  'last7Days',
  'last30Days',
  'older',
]

/**
 * i18n 键。`t(CONVERSATION_BUCKET_LABEL_KEY[bucket])` 得到分组标题。
 * 字面量同时也是 en locale 的键，与项目既有文案风格一致。
 */
export const CONVERSATION_BUCKET_LABEL_KEY: Record<
  ConversationDateBucket,
  string
> = {
  today: 'Today',
  yesterday: 'Yesterday',
  last7Days: 'Previous 7 Days',
  last30Days: 'Previous 30 Days',
  older: 'Older',
}

const MS_PER_DAY = 24 * 60 * 60 * 1000

/** 取本地时区下某时刻所在自然日的零点（毫秒）。 */
export function startOfLocalDay(timestamp: number): number {
  const d = new Date(timestamp)
  d.setHours(0, 0, 0, 0)
  return d.getTime()
}

/**
 * 判定时刻属于哪个分组。
 *
 * `now` 可注入以便测试（生产不传，用当前时间）——不注入的话用例只能靠
 * 「构造相对当前时间的偏移」，跨零点运行时必然偶发失败。
 */
export function getConversationBucket(
  timestamp: number,
  now: number = Date.now()
): ConversationDateBucket {
  if (!Number.isFinite(timestamp)) {
    return 'older'
  }
  const today = startOfLocalDay(now)
  // 未来时间（时钟漂移/手改系统时间）按「今天」处理，不产生负偏移分组。
  if (timestamp >= today) {
    return 'today'
  }
  const dayDiff = Math.round((today - startOfLocalDay(timestamp)) / MS_PER_DAY)
  if (dayDiff <= 1) {
    return 'yesterday'
  }
  if (dayDiff <= 7) {
    return 'last7Days'
  }
  if (dayDiff <= 30) {
    return 'last30Days'
  }
  return 'older'
}

/**
 * 把会话按时间分组。
 *
 * 每组内按 `updatedAt` **降序**（最近聊过的排最前）——这是用户找「刚聊过的那个」
 * 的最优顺序。空分组不产出，避免侧边栏出现没有内容的标题。
 */
export function groupConversationsByDate<
  T extends { id: string; updatedAt: number },
>(
  conversations: T[],
  now: number = Date.now()
): Array<{ bucket: ConversationDateBucket; conversations: T[] }> {
  const buckets = new Map<ConversationDateBucket, T[]>()
  for (const conversation of conversations) {
    const bucket = getConversationBucket(conversation.updatedAt, now)
    const existing = buckets.get(bucket)
    if (existing) {
      existing.push(conversation)
    } else {
      buckets.set(bucket, [conversation])
    }
  }
  return CONVERSATION_BUCKET_ORDER.flatMap((bucket) => {
    const items = buckets.get(bucket)
    if (!items || items.length === 0) {
      return []
    }
    return [
      {
        bucket,
        conversations: [...items].sort((a, b) => b.updatedAt - a.updatedAt),
      },
    ]
  })
}

/**
 * 用会话标题做大小写不敏感的子串过滤。
 *
 * 空查询返回原列表（不返回空）——侧边栏搜索框清空时应恢复全部会话，
 * 这是搜索框的基本预期。
 */
export function filterConversationsByTitle<T extends { title: string }>(
  conversations: T[],
  query: string
): T[] {
  const trimmed = query.trim().toLowerCase()
  if (trimmed === '') {
    return conversations
  }
  return conversations.filter((c) => c.title.toLowerCase().includes(trimmed))
}
