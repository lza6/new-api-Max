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
import { Brain, Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { updateUserMemoryInjection } from '../api'
import { parseUserSettings } from '../lib'
import type { UserProfile } from '../types'

/** 与服务端 relaykit/dto.MaxMemoryInjectionRunes 保持一致。 */
export const MAX_MEMORY_INJECTION_RUNES = 2000

type MemoryPreferencesCardProps = {
  profile: UserProfile | null
  onProfileUpdate: () => void
  /** 部署是否开启记忆注入（未开启时保存不生效，故不显示本卡片）。 */
  enabled: boolean
}

/**
 * T8 记忆注入配置卡片。
 *
 * 用户在此写下希望模型长期记住的偏好/背景，开启记忆注入的部署会把它作为
 * 稳定前缀注入到每次请求的 system 消息之前（利于上游前缀缓存）。
 *
 * 未开启时返回 null —— 不给用户一个「看起来能存但实际没作用」的开关。
 */
export function MemoryPreferencesCard(props: MemoryPreferencesCardProps) {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const [saving, setSaving] = useState(false)

  const savedMemory = useMemo(
    () => parseUserSettings(props.profile?.setting).memory_injection ?? '',
    [props.profile?.setting]
  )
  const [text, setText] = useState(savedMemory)

  useEffect(() => {
    setText(savedMemory)
  }, [savedMemory])

  if (!props.enabled) {
    return null
  }

  // 按 Unicode 码点计数（与后端 utf8.RuneCountInString 同口径），
  // 否则中文/emoji 会被 length 高估。
  const used = [...text].length
  const overLimit = used > MAX_MEMORY_INJECTION_RUNES
  const dirty = text !== savedMemory

  const handleSave = async () => {
    if (overLimit || !dirty) {
      return
    }
    setSaving(true)
    try {
      const response = await updateUserMemoryInjection(text)
      if (!response.success) {
        throw createServerError(response, t('Failed to update settings'))
      }

      // 同步本地 auth 缓存里的设置，避免其它读取设置的地方拿到旧值。
      if (auth.user) {
        const existingSetting =
          typeof auth.user.setting === 'string'
            ? parseUserSettings(auth.user.setting)
            : (auth.user.setting ?? {})
        auth.setUser({
          ...auth.user,
          setting: JSON.stringify({
            ...existingSetting,
            memory_injection: text,
          }),
        })
      }

      props.onProfileUpdate()
      toast.success(t('Memory saved'))
    } catch (error) {
      handleServerError(error, t('Failed to update settings'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <TitledCard
      title={t('My Memory')}
      description={t(
        'Details the model should remember about you across conversations'
      )}
      icon={<Brain className='h-4 w-4' />}
      iconTone='chart-4'
      disableHoverEffect
    >
      <div className='space-y-3'>
        <Textarea
          value={text}
          onChange={(event) => setText(event.target.value)}
          rows={5}
          maxLength={MAX_MEMORY_INJECTION_RUNES}
          aria-label={t('My Memory')}
          aria-invalid={overLimit}
          placeholder={t(
            'For example: I work in Go and React. Answer in Simplified Chinese, prefer concise replies with code examples.'
          )}
          disabled={saving}
        />
        <div className='flex items-start justify-between gap-3'>
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'This text is sent to the model before each conversation. Keep it short and stable.'
            )}
          </p>
          <span
            className='text-muted-foreground shrink-0 font-mono text-xs tabular-nums'
            aria-live='polite'
          >
            {used} / {MAX_MEMORY_INJECTION_RUNES}
          </span>
        </div>
        <div className='flex justify-end'>
          <Button onClick={handleSave} disabled={saving || overLimit || !dirty}>
            {saving && <Loader2 className='mr-1.5 size-3.5 animate-spin' />}
            {t('Save')}
          </Button>
        </div>
      </div>
    </TitledCard>
  )
}
