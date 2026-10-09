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
import { Loader2, Plus, Sparkles, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
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
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import { updateUserSkills } from '../api'
import { parseUserSettings } from '../lib'
import {
  MAX_SKILL_NAME_RUNES,
  MAX_SKILL_PROMPT_RUNES,
  MAX_USER_SKILLS,
  type UserProfile,
  type UserSkill,
} from '../types'

type SkillsPreferencesCardProps = {
  profile: UserProfile | null
  onProfileUpdate: () => void
  /** 部署是否开启注入（与记忆注入同一开关）。未开启时保存不生效，故不显示。 */
  enabled: boolean
}

type Draft = {
  id: string
  name: string
  prompt: string
}

function newDraft(): Draft {
  return {
    id:
      typeof crypto !== 'undefined' && 'randomUUID' in crypto
        ? crypto.randomUUID()
        : `skill-${Date.now()}`,
    name: '',
    prompt: '',
  }
}

/**
 * T13 Skills 注入 v1 配置卡片。
 *
 * 用户在此维护一组「技能」（名称 + 指令），启用中的技能会被拼进发往上游的
 * system 前缀，让模型按既定方式作答。与记忆注入共用同一条注入管线和同一个
 * 部署开关，因此未开启时**不渲染本卡片**（不做「看起来能用实际无效」的开关）。
 *
 * 整表覆盖提交：任何增删改都提交完整数组，避免局部合并带来的状态分叉。
 */
export function SkillsPreferencesCard(props: SkillsPreferencesCardProps) {
  const { t } = useTranslation()
  const { auth } = useAuthStore()
  const [saving, setSaving] = useState(false)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [pendingDelete, setPendingDelete] = useState<UserSkill | null>(null)

  const savedSkills = useMemo<UserSkill[]>(
    () => parseUserSettings(props.profile?.setting).skills ?? [],
    [props.profile?.setting]
  )
  const [skills, setSkills] = useState<UserSkill[]>(savedSkills)

  useEffect(() => {
    setSkills(savedSkills)
  }, [savedSkills])

  if (!props.enabled) {
    return null
  }

  const usedName = draft ? [...draft.name].length : 0
  const usedPrompt = draft ? [...draft.prompt].length : 0
  const draftInvalid =
    !draft ||
    draft.name.trim() === '' ||
    draft.prompt.trim() === '' ||
    usedName > MAX_SKILL_NAME_RUNES ||
    usedPrompt > MAX_SKILL_PROMPT_RUNES

  const persist = async (next: UserSkill[], successKey: string) => {
    setSaving(true)
    try {
      const response = await updateUserSkills(next)
      if (!response.success) {
        throw createServerError(response, t('Failed to update settings'))
      }
      // 同步本地 auth 缓存，避免其它读取设置的地方拿到旧值。
      if (auth.user) {
        const existing =
          typeof auth.user.setting === 'string'
            ? parseUserSettings(auth.user.setting)
            : (auth.user.setting ?? {})
        auth.setUser({
          ...auth.user,
          setting: JSON.stringify({ ...existing, skills: next }),
        })
      }
      setSkills(next)
      props.onProfileUpdate()
      toast.success(t(successKey))
      return true
    } catch (error) {
      handleServerError(error, t('Failed to update settings'))
      return false
    } finally {
      setSaving(false)
    }
  }

  const handleAdd = async () => {
    if (!draft || draftInvalid) {
      return
    }
    if (skills.length >= MAX_USER_SKILLS) {
      toast.error(t('You can add at most {{n}} skills', { n: MAX_USER_SKILLS }))
      return
    }
    const next = [
      ...skills,
      {
        id: draft.id,
        name: draft.name.trim(),
        prompt: draft.prompt.trim(),
        enabled: true,
      },
    ]
    if (await persist(next, 'Skill saved')) {
      setDraft(null)
    }
  }

  const handleToggle = (skill: UserSkill, enabled: boolean) => {
    void persist(
      skills.map((s) => (s.id === skill.id ? { ...s, enabled } : s)),
      'Skill saved'
    )
  }

  const handleDelete = async () => {
    if (!pendingDelete) {
      return
    }
    const target = pendingDelete
    setPendingDelete(null)
    await persist(
      skills.filter((s) => s.id !== target.id),
      'Skill deleted'
    )
  }

  return (
    <TitledCard
      title={t('My Skills')}
      description={t(
        'Instructions the model should follow in every conversation where relevant'
      )}
      icon={<Sparkles className='h-4 w-4' />}
      iconTone='chart-3'
      disableHoverEffect
    >
      <div className='space-y-3'>
        {skills.length === 0 ? (
          <EmptyState
            icon={Sparkles}
            title={t('No skills yet')}
            description={t(
              'Skills are short instructions, for example “always answer with a one-line summary first”.'
            )}
          />
        ) : (
          <ul className='divide-border/60 divide-y'>
            {skills.map((skill) => (
              <li
                key={skill.id}
                className='flex items-start gap-3 py-2.5 first:pt-0'
              >
                <Switch
                  checked={skill.enabled}
                  onCheckedChange={(checked) => handleToggle(skill, checked)}
                  disabled={saving}
                  aria-label={t('Enable {{name}}', { name: skill.name })}
                />
                <div className='min-w-0 flex-1'>
                  <div className='truncate text-sm font-medium'>
                    {skill.name}
                  </div>
                  <p className='text-muted-foreground line-clamp-2 text-xs break-words'>
                    {skill.prompt}
                  </p>
                </div>
                <Button
                  variant='ghost'
                  size='sm'
                  className='text-muted-foreground hover:text-destructive size-7 shrink-0 p-0'
                  onClick={() => setPendingDelete(skill)}
                  disabled={saving}
                  aria-label={t('Delete {{name}}', { name: skill.name })}
                >
                  <Trash2 className='size-3.5' />
                </Button>
              </li>
            ))}
          </ul>
        )}

        <div className='flex items-center justify-between gap-3'>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {skills.length} / {MAX_USER_SKILLS}
          </span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => setDraft(newDraft())}
            disabled={saving || skills.length >= MAX_USER_SKILLS}
          >
            {saving ? (
              <Loader2 className='mr-1.5 size-3.5 animate-spin' />
            ) : (
              <Plus className='mr-1.5 size-3.5' />
            )}
            {t('Add skill')}
          </Button>
        </div>
      </div>

      <Dialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDraft(null)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Add skill')}</DialogTitle>
            <DialogDescription>
              {t(
                'A skill is a short, reusable instruction the model will follow.'
              )}
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-3'>
            <div className='space-y-1.5'>
              <Label htmlFor='skill-name'>{t('Name')}</Label>
              <Input
                id='skill-name'
                value={draft?.name ?? ''}
                maxLength={MAX_SKILL_NAME_RUNES}
                onChange={(event) =>
                  setDraft((prev) =>
                    prev ? { ...prev, name: event.target.value } : prev
                  )
                }
                placeholder={t('Short, for example “Summary first”')}
              />
            </div>
            <div className='space-y-1.5'>
              <Label htmlFor='skill-prompt'>{t('Instruction')}</Label>
              <Textarea
                id='skill-prompt'
                rows={4}
                value={draft?.prompt ?? ''}
                maxLength={MAX_SKILL_PROMPT_RUNES}
                onChange={(event) =>
                  setDraft((prev) =>
                    prev ? { ...prev, prompt: event.target.value } : prev
                  )
                }
                placeholder={t(
                  'For example: start with a one-sentence conclusion, then the reasoning.'
                )}
              />
              <p className='text-muted-foreground text-right font-mono text-xs tabular-nums'>
                {usedPrompt} / {MAX_SKILL_PROMPT_RUNES}
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setDraft(null)}>
              {t('Cancel')}
            </Button>
            <Button onClick={handleAdd} disabled={draftInvalid || saving}>
              {saving && <Loader2 className='mr-1.5 size-3.5 animate-spin' />}
              {t('Save')}
            </Button>
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
        title={t('Delete skill')}
        desc={t('Delete the skill “{{name}}”? This cannot be undone.', {
          name: pendingDelete?.name ?? '',
        })}
        destructive
        handleConfirm={handleDelete}
      />
    </TitledCard>
  )
}
