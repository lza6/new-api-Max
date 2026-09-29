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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

interface AddMultiKeysDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (keys: string[]) => Promise<void> | void
}

/**
 * 新增密钥弹窗：每行一个密钥，提交时按行拆分（去空、去重由后端处理）。
 */
export function AddMultiKeysDialog(props: AddMultiKeysDialogProps) {
  const { t } = useTranslation()
  const [raw, setRaw] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const keys = raw
    .split('\n')
    .map((k) => k.trim())
    .filter((k) => k.length > 0)

  const handleSubmit = async () => {
    if (keys.length === 0) {return}
    setSubmitting(true)
    try {
      await props.onSubmit(keys)
      setRaw('')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {setRaw('')}
        props.onOpenChange(open)
      }}
      title={t('Add Keys')}
      description={t(
        'Paste one API key per line. Duplicate keys are skipped automatically.'
      )}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-3'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={submitting}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={submitting || keys.length === 0}
          >
            {t('Add {{count}} Keys', { count: keys.length })}
          </Button>
        </>
      }
    >
      <div className='space-y-2'>
        <Label htmlFor='add-multi-keys-input'>{t('API Keys')}</Label>
        <Textarea
          id='add-multi-keys-input'
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          placeholder={'sk-...\nsk-...'}
          className='min-h-[140px] font-mono text-xs'
          rows={6}
        />
        <p className='text-muted-foreground text-xs'>
          {t('{{count}} key(s) will be added', { count: keys.length })}
        </p>
      </div>
    </Dialog>
  )
}
