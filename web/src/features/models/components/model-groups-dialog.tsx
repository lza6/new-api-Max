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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog } from '@/components/dialog'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import { updateModelGroups } from '../api'

interface ModelGroupsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  modelName: string
  currentGroups: string[]
  availableGroups: string[]
  onSaved?: () => void
}

/**
 * T2：模型分组归类对话框。多选该模型应归属的分组，保存后写入 Model.Groups
 * 并同步到含该模型的渠道 group（并集）与 abilities，使模型在指定分组真实可用。
 */
export function ModelGroupsDialog({
  open,
  onOpenChange,
  modelName,
  currentGroups,
  availableGroups,
  onSaved,
}: ModelGroupsDialogProps) {
  const { t } = useTranslation()
  const [selected, setSelected] = useState<string[]>(currentGroups)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (open) setSelected(currentGroups)
  }, [open, currentGroups])

  const options = useMemo(() => {
    const seen = new Set<string>()
    return [...availableGroups, ...currentGroups].filter((g) => {
      if (!g || seen.has(g)) return false
      seen.add(g)
      return true
    })
  }, [availableGroups, currentGroups])

  const toggle = (group: string) => {
    setSelected((prev) =>
      prev.includes(group)
        ? prev.filter((g) => g !== group)
        : [...prev, group]
    )
  }

  const handleSave = async () => {
    if (!selected.length) {
      toast.error(t('Select at least one group'))
      return
    }
    setSaving(true)
    try {
      const res = await updateModelGroups({ model_name: modelName, groups: selected })
      if (!res.success) {
        handleServerError(res, t('Failed to update model groups'))
        return
      }
      const updated = res.data?.updated_channels ?? 0
      toast.success(
        t('Model groups updated ({{count}} channels synced)', {
          count: updated,
        })
      )
      onOpenChange(false)
      onSaved?.()
    } catch (error) {
      handleServerError(error, t('Failed to update model groups'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Assign model groups')}
      description={t('Model {{name}} will be available in the selected groups', {
        name: modelName,
      })}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      footer={
        <div className='flex items-center justify-end gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button size='sm' onClick={handleSave} disabled={saving}>
            {saving ? t('Saving…') : t('Save')}
          </Button>
        </div>
      }
    >
      <div className='grid max-h-[50vh] gap-2 overflow-auto pr-1'>
        {options.map((group) => {
          const checked = selected.includes(group)
          return (
            <label
              key={group}
              className='hover:bg-muted/50 flex items-center gap-3 rounded-lg border px-3 py-2.5 text-sm transition-colors'
            >
              <Checkbox checked={checked} onCheckedChange={() => toggle(group)} />
              <span className='font-medium'>{group}</span>
              <span className='text-muted-foreground text-xs'>
                {t('Usable group')}
              </span>
            </label>
          )
        })}
        {options.length === 0 && (
          <p className='text-muted-foreground py-4 text-center text-sm'>
            {t('No groups available')}
          </p>
        )}
      </div>
      <p className='text-muted-foreground/70 mt-3 text-xs'>
        {t(
          'Saving also adds these groups to every enabled channel serving this model, so it becomes callable under them.'
        )}
      </p>
      <Label className='sr-only'>{t('Assign model groups')}</Label>
    </Dialog>
  )
}
