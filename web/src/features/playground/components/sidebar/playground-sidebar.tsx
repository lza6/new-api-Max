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
// 游乐场会话侧边栏（T1）：新建 / 按时间分组浏览 / 搜索 / 折叠 / 删除。
//
// ## 为什么替换原来的下拉
//
// 原来只有 `conversation-switcher.tsx` 的一个 `<Select>`：会话多了以后无法浏览、
// 无法按时间定位、没有搜索、删除只能在下拉里点。用户明确要求参考项目那种
// 「左侧会话列表 + 折叠」的形态。
//
// ## 复用而非另造
//
// - `ConfirmDialog`（项目共享的危险操作确认封装）——删除会话是不可逆的。
// - `Button` / `Input` / `ScrollArea` / `Tooltip`（`@/components/ui/*`）。
// - 分组与过滤是**纯函数**，放在 `lib/conversations/conversation-grouping.ts`，
//   因此可被单测覆盖；本组件只负责渲染与交互。
import { useMemo, useState } from 'react'
import {
  MessageSquarePlus,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import {
  CONVERSATION_BUCKET_LABEL_KEY,
  filterConversationsByTitle,
  groupConversationsByDate,
} from '../../lib/conversations/conversation-grouping'
import type { Conversation } from '../../lib'

type PlaygroundSidebarProps = {
  conversations: Conversation[]
  activeConversationId: string
  collapsed: boolean
  disabled?: boolean
  onCreate: () => void
  onDelete: (id: string) => void
  onSwitch: (id: string) => void
  onToggleCollapsed: () => void
}

export function PlaygroundSidebar(props: PlaygroundSidebarProps) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [pendingDelete, setPendingDelete] = useState<Conversation | null>(null)

  // 分组依赖 updatedAt，只随会话列表变化重算；搜索词变化只影响过滤结果。
  const groups = useMemo(() => {
    const filtered = filterConversationsByTitle(props.conversations, query)
    return groupConversationsByDate(filtered)
  }, [props.conversations, query])

  // 折叠态：只留一条窄栏与「展开/新建」按钮，把正文让给对话。
  if (props.collapsed) {
    return (
      <div className='border-border/60 flex h-full w-12 shrink-0 flex-col items-center gap-2 border-r py-2'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                aria-label={t('Expand sidebar')}
                size='icon'
                variant='ghost'
                className='size-8'
                disabled={props.disabled}
                onClick={props.onToggleCollapsed}
              >
                <PanelLeftOpen className='size-4' />
              </Button>
            }
          />
          <TooltipContent>{t('Expand sidebar')}</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                aria-label={t('New chat')}
                size='icon'
                variant='ghost'
                className='size-8'
                disabled={props.disabled}
                onClick={props.onCreate}
              >
                <MessageSquarePlus className='size-4' />
              </Button>
            }
          />
          <TooltipContent>{t('New chat')}</TooltipContent>
        </Tooltip>
      </div>
    )
  }

  return (
    <aside
      aria-label={t('Conversations')}
      className='border-border/60 flex h-full w-64 shrink-0 flex-col border-r'
    >
      {/* 顶部操作区：新建 + 折叠。新建放在最显眼处（参考项目同样把「开启新对话」
          做成整行按钮，而不是塞进菜单）。 */}
      <div className='flex items-center gap-1 p-2'>
        <Button
          variant='outline'
          size='sm'
          className='h-8 flex-1 justify-start gap-2'
          disabled={props.disabled}
          onClick={props.onCreate}
        >
          <MessageSquarePlus className='size-3.5' />
          {t('New chat')}
        </Button>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                aria-label={t('Collapse sidebar')}
                size='icon'
                variant='ghost'
                className='size-8'
                disabled={props.disabled}
                onClick={props.onToggleCollapsed}
              >
                <PanelLeftClose className='size-4' />
              </Button>
            }
          />
          <TooltipContent>{t('Collapse sidebar')}</TooltipContent>
        </Tooltip>
      </div>

      <div className='relative px-2 pb-1'>
        <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-4 size-3.5 -translate-y-1/2' />
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('Search conversations...')}
          aria-label={t('Search conversations...')}
          className='h-8 pl-7 text-xs'
        />
      </div>

      <ScrollArea className='min-h-0 flex-1'>
        <div className='p-2 pt-1'>
          {groups.length === 0 ? (
            <p className='text-muted-foreground px-2 py-6 text-center text-xs'>
              {t('No conversations found.')}
            </p>
          ) : (
            groups.map((group) => (
              <div key={group.bucket} className='mb-3'>
                <div className='text-muted-foreground px-2 py-1 text-[10px] font-medium tracking-wide uppercase'>
                  {t(CONVERSATION_BUCKET_LABEL_KEY[group.bucket])}
                </div>
                {group.conversations.map((conversation) => {
                  const isActive = conversation.id === props.activeConversationId
                  return (
                    <div
                      key={conversation.id}
                      className={cn(
                        'group/row mb-0.5 flex items-center gap-1 rounded-md',
                        isActive ? 'bg-accent' : 'hover:bg-accent/60'
                      )}
                    >
                      <button
                        type='button'
                        // 会话标题本身就是可访问名称，不再额外加 aria-label
                        // （否则屏幕阅读器会读两遍）。
                        aria-current={isActive ? 'true' : undefined}
                        disabled={props.disabled}
                        onClick={() => props.onSwitch(conversation.id)}
                        className='min-w-0 flex-1 truncate px-2 py-1.5 text-left text-xs disabled:opacity-50'
                        title={conversation.title}
                      >
                        {conversation.title}
                      </button>
                      {/* 删除按钮 hover/聚焦才出现：常显会让列表噪声过大，
                          但必须能键盘到达（group-focus-within）。 */}
                      <Button
                        aria-label={t('Delete conversation')}
                        size='icon'
                        variant='ghost'
                        disabled={props.disabled}
                        onClick={() => setPendingDelete(conversation)}
                        className='mr-1 size-6 opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 group-focus-within/row:opacity-100'
                      >
                        <Trash2 className='size-3.5' />
                      </Button>
                    </div>
                  )
                })}
              </div>
            ))
          )}
        </div>
      </ScrollArea>

      {/* 删除会话不可逆：走项目共享的 ConfirmDialog，而不是直接删。 */}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null)
          }
        }}
        title={t('Delete conversation')}
        desc={
          pendingDelete
            ? t('Delete "{{title}}"? This cannot be undone.', {
                title: pendingDelete.title,
              })
            : ''
        }
        confirmText={t('Delete')}
        destructive
        handleConfirm={() => {
          if (pendingDelete) {
            props.onDelete(pendingDelete.id)
          }
          setPendingDelete(null)
        }}
      />
    </aside>
  )
}
