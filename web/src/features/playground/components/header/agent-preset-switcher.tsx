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
// T13 per-agent 能力下沉 v1：Agent 预设切换器。
//
// 「预设」= 一组可复用的对话配置（模型/分组/系统提示词/参数）。用户在游乐场
// 一键套用，把「助手」的配置与聊天记录解耦。
//
// 存储：与记忆/技能同一条管线——用户设置的 JSON 列（`agent_presets`），
// 经 PUT /api/user/self 整表覆盖。因此这里直接读 profile，不引入新的后端端点。
import { Check, Loader2, Save, Settings2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { updateAgentPresets } from '@/features/profile/api'
import { parseUserSettings } from '@/features/profile/lib'

import {
  applyPresetToConfig,
  presetFromConfig,
  validatePresetList,
  type PresetValidationError,
} from '../../lib/presets/agent-preset-utils'
import type { AgentPreset, PlaygroundConfig } from '../../types'
import { AgentPresetsDialog } from './agent-presets-dialog'

type AgentPresetSwitcherProps = {
  config: PlaygroundConfig
  disabled?: boolean
  /** 来自 useProfile 的刷新回调；保存预设后刷新，使设置各处一致。 */
  onProfileUpdate?: () => void
  /** profile.setting 原文（用于解析已存预设）。 */
  setting?: string
  /** 当前选中的预设 id（由父组件持有，可跨渲染保持）。 */
  activePresetId?: string
  onActivePresetChange?: (id: string) => void
  /** 应用预设时把算好的新配置交给父组件落地（父组件调用 updateConfig）。 */
  onApplyPreset?: (next: PlaygroundConfig) => void
}

/**
 * 把校验错误码翻译为可展示的文案（i18n 由调用方提供 t）。
 */
function presetErrorKey(
  error: PresetValidationError,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  switch (error) {
    case 'name_required':
      return t('Preset name is required')
    case 'name_too_long':
      return t('Preset name is too long')
    case 'system_prompt_too_long':
      return t('System prompt is too long')
    case 'temperature_out_of_range':
      return t('Temperature must be between 0 and 2')
    case 'max_tokens_out_of_range':
      return t('Max tokens must be between 1 and {{n}}', { n: 32000 })
    case 'too_many_presets':
      return t('You can save at most {{n}} presets', { n: 20 })
    default:
      return t('Failed to update settings')
  }
}

export function AgentPresetSwitcher(props: AgentPresetSwitcherProps) {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const [saving, setSaving] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)

  const presets = useMemo<AgentPreset[]>(
    () => parseUserSettings(props.setting).agent_presets ?? [],
    [props.setting]
  )

  const persist = async (next: AgentPreset[]) => {
    const error = validatePresetList(next)
    if (error) {
      toast.error(presetErrorKey(error, t))
      return false
    }
    setSaving(true)
    try {
      const response = await updateAgentPresets(next)
      if (!response.success) {
        throw createServerError(response, t('Failed to update settings'))
      }
      if (auth.user) {
        const existing =
          typeof auth.user.setting === 'string'
            ? parseUserSettings(auth.user.setting)
            : (auth.user.setting ?? {})
        auth.setUser({
          ...auth.user,
          setting: JSON.stringify({ ...existing, agent_presets: next }),
        })
      }
      props.onProfileUpdate?.()
      return true
    } catch (err) {
      handleServerError(err, t('Failed to update settings'))
      return false
    } finally {
      setSaving(false)
    }
  }

  const handleApply = (id: string) => {
    const preset = presets.find((p) => p.id === id)
    if (!preset) {
      return
    }
    props.onActivePresetChange?.(id)
    // 父组件负责把新配置写回（updateConfig），本组件不直接持有 config 状态。
    props.onApplyPreset?.(applyPresetToConfig(props.config, preset))
    toast.success(t('Preset applied'))
  }

  const handleSaveCurrent = async () => {
    const name = t('Preset {{n}}', { n: presets.length + 1 })
    const next = [...presets, presetFromConfig(props.config, name)]
    if (await persist(next)) {
      toast.success(t('Preset saved'))
    }
  }

  return (
    <div className='flex items-center gap-2'>
      <Select
        value={props.activePresetId ?? ''}
        onValueChange={(value) => {
          if (value) {
            handleApply(value)
          }
        }}
      >
        <SelectTrigger
          aria-label={t('Agent preset')}
          className='h-8 max-w-44 min-w-0 text-sm'
          disabled={props.disabled || saving}
        >
          <SelectValue placeholder={t('No preset')} />
        </SelectTrigger>
        <SelectContent className='max-w-[calc(100vw-2rem)] min-w-44'>
          {presets.length === 0 ? (
            <div className='text-muted-foreground px-2 py-3 text-center text-xs'>
              {t('No presets yet')}
            </div>
          ) : (
            presets.map((preset) => (
              <SelectItem key={preset.id} value={preset.id}>
                <span className='flex items-center gap-1.5 truncate'>
                  {preset.id === props.activePresetId && (
                    <Check className='size-3.5 shrink-0' />
                  )}
                  <span className='truncate'>{preset.name}</span>
                </span>
              </SelectItem>
            ))
          )}
        </SelectContent>
      </Select>

      <Button
        aria-label={t('Save current setup as a preset')}
        disabled={props.disabled || saving}
        onClick={handleSaveCurrent}
        size='icon'
        variant='ghost'
        title={t('Save current setup as a preset')}
      >
        {saving ? (
          <Loader2 className='size-4 animate-spin' />
        ) : (
          <Save className='text-muted-foreground size-4' />
        )}
      </Button>

      <Button
        aria-label={t('Manage presets')}
        disabled={props.disabled}
        onClick={() => setDialogOpen(true)}
        size='icon'
        variant='ghost'
        title={t('Manage presets')}
      >
        <Settings2 className='text-muted-foreground size-4' />
      </Button>

      <AgentPresetsDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        presets={presets}
        saving={saving}
        onPersist={persist}
      />
    </div>
  )
}

