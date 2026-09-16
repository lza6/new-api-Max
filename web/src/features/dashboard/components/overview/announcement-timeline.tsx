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
import { History } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { getPreviewText } from '@/features/dashboard/lib'
import type { AnnouncementItem } from '@/features/dashboard/types'
import { getAnnouncementColorClass } from '@/lib/colors'
import { formatDateTimeObject } from '@/lib/time'
import { cn } from '@/lib/utils'

interface AnnouncementTimelineProps {
  items: AnnouncementItem[]
  onSelect: (item: AnnouncementItem) => void
}

/**
 * T4：历史公告时间线。把所有公告（已按 publishDate 降序）渲染为垂直时间轴，
 * 时间点 + 类型色点 + 内容摘要，点击查看详情。
 */
export function AnnouncementTimeline({
  items,
  onSelect,
}: AnnouncementTimelineProps) {
  const { t } = useTranslation()

  if (items.length === 0) {
    return null
  }

  return (
    <ScrollArea className='h-72 pr-3'>
      <ol className='relative ml-2 border-l border-border pl-6'>
        {items.map((item, idx) => {
          const key = item.id ?? `announcement-${idx}`
          const isLast = idx === items.length - 1
          return (
            <li
              key={key}
              className={cn('relative pb-6', isLast && 'pb-0')}
            >
              {/* 时间轴上的类型色点 */}
              <span
                aria-hidden
                className={cn(
                  'absolute top-1.5 -left-[31px] size-2.5 rounded-full border-2 border-background',
                  getAnnouncementColorClass(item.type)
                )}
              />
              <button
                type='button'
                onClick={() => onSelect(item)}
                className='group hover:bg-muted/40 -mx-2 flex w-[calc(100%+1rem)] flex-col gap-1 rounded-lg px-2 py-1 text-left transition-colors'
              >
                <div className='flex items-center justify-between gap-2'>
                  <time className='text-muted-foreground/70 text-xs font-medium tabular-nums'>
                    {item.publishDate
                      ? formatDateTimeObject(new Date(item.publishDate))
                      : '—'}
                  </time>
                  <span className='text-muted-foreground/50 text-[11px] opacity-0 transition-opacity group-hover:opacity-100'>
                    {t('Click for details')}
                  </span>
                </div>
                <p className='line-clamp-2 text-sm text-balance'>
                  {getPreviewText(item.content)}
                </p>
                {item.extra && (
                  <p className='text-muted-foreground line-clamp-1 text-xs'>
                    {item.extra}
                  </p>
                )}
              </button>
            </li>
          )
        })}
      </ol>
    </ScrollArea>
  )
}

/** 时间线面板标题（带图标）。 */
export function TimelinePanelHeader() {
  const { t } = useTranslation()
  return (
    <span className='flex items-center gap-2'>
      <IconBadge tone='info' size='sm'>
        <History />
      </IconBadge>
      {t('Announcement Timeline')}
    </span>
  )
}