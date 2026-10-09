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
// T13 Agent 预设管理对话框：列出 / 编辑 / 删除预设。
//
// 保存始终提交**完整数组**（整表覆盖），与后端语义一致；不做局部合并，
// 避免多端并发编辑时状态分叉。
import { Bot, Loader2, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import {
  MAX_AGENT_PRESETS,
  MAX_PRESET_NAME_RUNES,
  MAX_PRESET_SYSTEM_RUNES,
  presetFromConfig,
  runeLength,
  validatePreset,
  type PresetValidationError,
} from '../../lib/presets/agent-preset-utils'
import type { AgentPreset, PlaygroundConfig } from '../../types'

type AgentPresetsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  presets: AgentPreset[]
  saving: boolean
  onPersist: (next: AgentPreset[]) => Promise<boolean>
}

const EMPTY_CONFIG: PlaygroundConfig = {
  model: '',
  group: '',
  temperature: 0.7,
  top_p: 1,
  max_tokens: 4096,
  frequency_penalty: 0,
  presence_penalty: 0,
  seed: null,
  stream: true,
  size: '',
  reasoning_effort: '',
  system_prompt: '',
}

function errorText(
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
      return t('You can save at most {{n}} presets', { n: MAX_AGENT_PRESETS })
    default:
      return t('Failed to update settings')
  }
}

export function AgentPresetsDialog(props: AgentPresetsDialogProps) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<AgentPreset | null>(null)
  const [pendingDelete, setPendingDelete] = useState<AgentPreset | null>(null)

  const handleSaveEdit = async () => {
    if (!editing) {
      return
    }
    const normalized: AgentPreset = {
      ...editing,
      name: editing.name.trim(),
      system_prompt: editing.system_prompt?.trim() ?? '',
    }
    const error = validatePreset(normalized)
    if (error) {
      toast.error(errorText(error, t))
      return
    }
    const exists = props.presets.some((p) => p.id === normalized.id)
    const next = exists
      ? props.presets.map((p) => (p.id === normalized.id ? normalized : p))
      : [...props.presets, normalized]
    if (await props.onPersist(next)) {
      toast.success(t('Preset saved'))
      setEditing(null)
    }
  }

  const handleDelete = async () => {
    if (!pendingDelete) {
      return
    }
    const target = pendingDelete
    setPendingDelete(null)
    if (await props.onPersist(props.presets.filter((p) => p.id !== target.id))) {
      toast.success(t('Preset deleted'))
    }
  }

  const usedSystem = editing ? runeLength(editing.system_prompt ?? '') : 0

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{t('Manage presets')}</DialogTitle>
            <DialogDescription>
              {t(
                'A preset stores a model, parameters and system prompt, so you can switch between assistants in one click.'
              )}
            </DialogDescription>
          </DialogHeader>

          {editing ? (
            <div className='space-y-3'>
              <div className='space-y-1.5'>
                <Label htmlFor='preset-name'>{t('Name')}</Label>
                <Input
                  id='preset-name'
                  value={editing.name}
                  maxLength={MAX_PRESET_NAME_RUNES}
                  onChange={(event) =>
                    setEditing({ ...editing, name: event.target.value })
                  }
                />
              </div>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1.5'>
                  <Label htmlFor='preset-model'>{t('Model')}</Label>
                  <Input
                    id='preset-model'
                    value={editing.model ?? ''}
                    onChange={(event) =>
                      setEditing({ ...editing, model: event.target.value })
                    }
                    placeholder={t('Leave empty to keep the current model')}
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label htmlFor='preset-group'>{t('Group')}</Label>
                  <Input
                    id='preset-group'
                    value={editing.group ?? ''}
                    onChange={(event) =>
                      setEditing({ ...editing, group: event.target.value })
                    }
                    placeholder={t('Leave empty to keep the current group')}
                  />
                </div>
              </div>
              <div className='space-y-1.5'>
                <Label htmlFor='preset-system'>{t('System prompt')}</Label>
                <Textarea
                  id='preset-system'
                  rows={4}
                  value={editing.system_prompt ?? ''}
                  maxLength={MAX_PRESET_SYSTEM_RUNES}
                  onChange={(event) =>
                    setEditing({
                      ...editing,
                      system_prompt: event.target.value,
                    })
                  }
                  placeholder={t(
                    'For example: you are a senior Go engineer. Prefer concise answers with runnable code.'
                  )}
                />
                <p className='text-muted-foreground text-right font-mono text-xs tabular-nums'>
                  {usedSystem} / {MAX_PRESET_SYSTEM_RUNES}
                </p>
              </div>
            </div>
          ) : (
            <div className='space-y-2'>
              {props.presets.length === 0 ? (
                <p className='text-muted-foreground py-6 text-center text-sm'>
                  {t('No presets yet')}
                </p>
              ) : (
                <ul className='divide-border/60 divide-y'>
                  {props.presets.map((preset) => (
                    <li
                      key={preset.id}
                      className='flex items-center gap-3 py-2'
                    >
                      <Bot className='text-muted-foreground size-4 shrink-0' />
                      <div className='min-w-0 flex-1'>
                        <div className='truncate text-sm font-medium'>
                          {preset.name}
                        </div>
                        <p className='text-muted-foreground truncate text-xs'>
                          {preset.model || t('Current model')}
                          {preset.system_prompt
                            ? ` · ${preset.system_prompt}`
                            : ''}
                        </p>
                      </div>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => setEditing(preset)}
                      >
                        {t('Edit')}
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        className='text-muted-foreground hover:text-destructive size-7 p-0'
                        onClick={() => setPendingDelete(preset)}
                        aria-label={t('Delete {{name}}', { name: preset.name })}
                      >
                        <Trash2 className='size-3.5' />
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}

          <DialogFooter className='sm:justify-between'>
            {editing ? (
              <>
                <Button variant='ghost' onClick={() => setEditing(null)}>
                  {t('Back')}
                </Button>
                <Button onClick={handleSaveEdit} disabled={props.saving}>
                  {props.saving && (
                    <Loader2 className='mr-1.5 size-3.5 animate-spin' />
                  )}
                  {t('Save')}
                </Button>
              </>
            ) : (
              <>
                <span className='text-muted-foreground text-xs tabular-nums'>
                  {props.presets.length} / {MAX_AGENT_PRESETS}
                </span>
                <Button
                  variant='outline'
                  disabled={
                    props.saving ||
                    props.presets.length >= MAX_AGENT_PRESETS
                  }
                  onClick={() =>
                    setEditing(
                      presetFromConfig(
                        EMPTY_CONFIG,
                        t('Preset {{n}}', { n: props.presets.length + 1 })
                      )
                    )
                  }
                >
                  <Plus className='mr-1.5 size-3.5' />
                  {t('Add preset')}
                </Button>
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null)
          }
        }}
        title={t('Delete preset')}
        desc={t('Delete the preset “{{name}}”? This cannot be undone.', {
          name: pendingDelete?.name ?? '',
        })}
        destructive
        handleConfirm={handleDelete}
      />
    </>
  )
}
