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
// P0-4 游乐场多会话：会话切换下拉（新建 / 切换 / 删除）。
// 排在输入框上方的工具条，避免占用正文空间。
import { MessageSquarePlus, Trash2Icon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Conversation } from '../../lib'

type ConversationSwitcherProps = {
  conversations: Conversation[]
  activeConversationId: string
  disabled?: boolean
  onCreate: () => void
  onDelete: (id: string) => void
  onSwitch: (id: string) => void
}

export function ConversationSwitcher({
  conversations,
  activeConversationId,
  disabled,
  onCreate,
  onDelete,
  onSwitch,
}: ConversationSwitcherProps) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center gap-2'>
      <Select
        value={activeConversationId}
        onValueChange={(value) => {
          if (value) {onSwitch(value)}
        }}
      >
        <SelectTrigger
          aria-label={t('Conversations')}
          className='h-8 max-w-56 min-w-0 text-sm'
          disabled={disabled}
        >
          <SelectValue placeholder={t('New chat')} />
        </SelectTrigger>
        <SelectContent className='max-w-[calc(100vw-2rem)] min-w-56'>
          {conversations.map((conversation) => (
            <SelectItem key={conversation.id} value={conversation.id}>
              <span className='truncate'>
                {conversation.title || t('New chat')}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Button
        aria-label={t('New chat')}
        disabled={disabled}
        onClick={onCreate}
        size='sm'
        variant='outline'
      >
        <MessageSquarePlus data-icon='inline-start' />
        <span className='hidden sm:inline'>{t('New chat')}</span>
      </Button>

      <Button
        aria-label={t('Delete conversation')}
        disabled={disabled || conversations.length <= 1}
        onClick={() => onDelete(activeConversationId)}
        size='icon'
        variant='ghost'
      >
        <Trash2Icon className='text-muted-foreground size-4' />
      </Button>
    </div>
  )
}
