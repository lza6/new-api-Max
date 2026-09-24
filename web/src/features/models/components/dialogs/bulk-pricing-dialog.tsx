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
*/
// 批量定价：管理员多选模型后一键设置 model_ratio / completion_ratio /
// cache_ratio。后端 saveModelPricing(changes[]) 已支持批量，乐观锁 version
// 由 getModelPricing(names) 逐模型获取（缺省用 empty_version）。
import { useMutation, useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  getModelPricing,
  saveModelPricing,
  useCanEditModelPricing,
} from '@/features/model-pricing/api'
import { handleServerError } from '@/lib/handle-server-error'

import type { Model } from '../../types'

interface BulkPricingDialogProps {
  models: Pick<Model, 'model_name'>[]
  onClose: () => void
  onSuccess?: () => void
}

export function BulkPricingDialog(props: BulkPricingDialogProps) {
  const { t } = useTranslation()
  const canEdit = useCanEditModelPricing()
  const [modelRatio, setModelRatio] = useState('')
  const [completionRatio, setCompletionRatio] = useState('')
  const [cacheRatio, setCacheRatio] = useState('')

  const names = useMemo(
    () => props.models.map((m) => m.model_name).filter(Boolean),
    [props.models]
  )

  // 批量前拉取各模型当前 version（乐观锁）；新模型用 empty_version。
  const pricingQuery = useQuery({
    queryKey: ['model-pricing-bulk', ...names],
    queryFn: () => getModelPricing(names),
    enabled: names.length > 0 && canEdit,
  })

  const save = useMutation({
    mutationFn: async () => {
      const entries = new Map(
        (pricingQuery.data?.entries ?? []).map((e) => [e.model_name, e])
      )
      const pricing: Record<string, number> = {}
      if (modelRatio !== '') {pricing['model_ratio'] = Number(modelRatio)}
      if (completionRatio !== '') {
        pricing['completion_ratio'] = Number(completionRatio)
      }
      if (cacheRatio !== '') {pricing['cache_ratio'] = Number(cacheRatio)}
      const changes = names.map((name) => ({
        model_name: name,
        expected_version:
          entries.get(name)?.version ?? pricingQuery.data?.empty_version ?? '',
        pricing,
      }))
      await saveModelPricing(changes)
    },
    onSuccess: () => {
      toast.success(t('Model pricing saved'))
      props.onSuccess?.()
      props.onClose()
    },
    onError: (error) => handleServerError(error),
  })

  if (!canEdit) {return null}

  return (
    <Dialog open onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent className='max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('Bulk set model pricing')}</DialogTitle>
          <DialogDescription>
            {t('Apply pricing to {{count}} selected models', {
              count: names.length,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <div className='space-y-2'>
            <Label htmlFor='bulk-model-ratio'>{t('Model ratio')}</Label>
            <Input
              id='bulk-model-ratio'
              type='number'
              step='0.01'
              min='0'
              placeholder={t('Leave empty to keep current')}
              value={modelRatio}
              onChange={(e) => setModelRatio(e.target.value)}
            />
          </div>
          <div className='space-y-2'>
            <Label htmlFor='bulk-completion-ratio'>
              {t('Completion ratio')}
            </Label>
            <Input
              id='bulk-completion-ratio'
              type='number'
              step='0.01'
              min='0'
              placeholder={t('Leave empty to keep current')}
              value={completionRatio}
              onChange={(e) => setCompletionRatio(e.target.value)}
            />
          </div>
          <div className='space-y-2'>
            <Label htmlFor='bulk-cache-ratio'>{t('Cache ratio')}</Label>
            <Input
              id='bulk-cache-ratio'
              type='number'
              step='0.01'
              min='0'
              placeholder={t('Leave empty to keep current')}
              value={cacheRatio}
              onChange={(e) => setCacheRatio(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button
            disabled={save.isPending || (!modelRatio && !completionRatio && !cacheRatio)}
            onClick={() => save.mutate()}
          >
            {save.isPending ? t('Saving...') : t('Apply to all selected')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}