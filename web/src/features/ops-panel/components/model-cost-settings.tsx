import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatLocalCurrencyAmount } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { createModelCost, getModelCosts } from '../api'
import { OPS_QUERY_KEY } from '../constants'
import type { ModelCostRule } from '../types'

function microsToDisplay(micros: number): string {
  if (!micros) {
    return '-'
  }
  return formatLocalCurrencyAmount(micros / 1e6, {
    digitsSmall: 6,
    minimumNonZero: 0.000001,
  })
}

interface Draft {
  channelId: string
  model: string
  mode: 'call' | 'token'
  perCall: string
  inputPerMillion: string
  outputPerMillion: string
  note: string
}

const emptyDraft: Draft = {
  channelId: '',
  model: '',
  mode: 'call',
  perCall: '',
  inputPerMillion: '',
  outputPerMillion: '',
  note: '',
}

function parseMoney(value: string): number {
  const n = Number.parseFloat(value.trim())
  return Number.isFinite(n) && n > 0 ? n : 0
}

/**
 * 上游成本设置（管理员专属，普通用户不可见）。
 *
 * 只**追加**历史：每次保存都会生成一条带生效时间的规则，旧规则保留。
 * 每条请求结算时按当时生效的规则把成本快照写进日志，所以调价不会改写历史利润。
 */
export function ModelCostSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [draft, setDraft] = useState<Draft>(emptyDraft)

  const query = useQuery({
    queryKey: [OPS_QUERY_KEY, 'model-costs'],
    queryFn: () => getModelCosts({ limit: 200 }),
    staleTime: 30_000,
    retry: false,
  })

  const mutation = useMutation({
    mutationFn: async () => {
      const channelId = Number.parseInt(draft.channelId, 10)
      if (!Number.isFinite(channelId) || channelId <= 0) {
        throw new Error(t('Channel ID is required'))
      }
      const perCall = parseMoney(draft.perCall)
      const input = parseMoney(draft.inputPerMillion)
      const output = parseMoney(draft.outputPerMillion)
      if (draft.mode === 'call' && perCall <= 0) {
        throw new Error(t('Per-call cost must be positive'))
      }
      if (draft.mode === 'token' && input <= 0 && output <= 0) {
        throw new Error(t('At least one token cost must be positive'))
      }
      await createModelCost({
        channel_id: channelId,
        model: draft.model.trim(),
        billing_mode: draft.mode,
        cost_per_call_cny: perCall,
        cost_input_per_million_cny: input,
        cost_output_per_million_cny: output,
        note: draft.note.trim(),
      })
    },
    onSuccess: () => {
      toast.success(t('Cost rule added'))
      setOpen(false)
      setDraft(emptyDraft)
      void queryClient.invalidateQueries({ queryKey: [OPS_QUERY_KEY] })
    },
    onError: (error) => handleServerError(error, t('Failed to save')),
  })

  const columns = useMemo<StaticDataTableColumn<ModelCostRule>[]>(
    () => [
      {
        id: 'channel',
        header: t('Channel'),
        cell: (row) => <span className='tabular-nums'>#{row.channel_id}</span>,
      },
      {
        id: 'model',
        header: t('Model'),
        cell: (row) => (
          <span className='font-mono text-xs'>
            {row.model || t('Channel default')}
          </span>
        ),
      },
      {
        id: 'mode',
        header: t('Billing Mode'),
        cell: (row) =>
          row.billing_mode === 'token' ? t('Per token') : t('Per call'),
      },
      {
        id: 'per_call',
        header: t('Cost per call'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <span className='tabular-nums'>
            {microsToDisplay(row.cost_per_call_micros)}
          </span>
        ),
      },
      {
        id: 'token_in',
        header: t('Input / 1M tokens'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <span className='tabular-nums'>
            {microsToDisplay(row.cost_input_per_million_micros)}
          </span>
        ),
      },
      {
        id: 'token_out',
        header: t('Output / 1M tokens'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row) => (
          <span className='tabular-nums'>
            {microsToDisplay(row.cost_output_per_million_micros)}
          </span>
        ),
      },
      {
        id: 'effective',
        header: t('Effective from'),
        cell: (row) => (
          <span className='text-muted-foreground text-xs'>
            {new Date(row.effective_from * 1000).toLocaleString()}
          </span>
        ),
      },
      {
        id: 'note',
        header: t('Note'),
        cell: (row) => (
          <span className='text-muted-foreground text-xs'>{row.note}</span>
        ),
      },
    ],
    [t]
  )

  const items = query.data?.items ?? []
  let emptyContent: ReactNode = t('No cost rules yet')
  if (query.isLoading) {
    emptyContent = <Skeleton className='mx-auto h-5 w-48' />
  } else if (query.isError) {
    emptyContent = t('Failed to load')
  }

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 sm:px-5'>
        <div className='min-w-0'>
          <h3 className='text-sm font-semibold'>{t('Model Costs')}</h3>
          <p className='text-muted-foreground mt-0.5 text-xs'>
            {t(
              'Upstream cost per channel/model, in CNY. Saving appends a new effective-dated rule; history is never rewritten.'
            )}
          </p>
        </div>
        <Button size='sm' onClick={() => setOpen(true)}>
          <Plus className='size-4' />
          {t('Add cost rule')}
        </Button>
      </div>

      <StaticDataTable<ModelCostRule>
        columns={columns}
        data={items}
        getRowKey={(row) => row.id}
        empty={items.length === 0}
        emptyContent={emptyContent}
      />

      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Add cost rule')}
        description={t(
          'Values are in CNY. The rule applies from now on; existing logs keep their original cost snapshots.'
        )}
        footer={
          <>
            <Button variant='outline' onClick={() => setOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button
              onClick={() => mutation.mutate()}
              disabled={mutation.isPending}
            >
              {t('Save')}
            </Button>
          </>
        }
      >
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='space-y-1.5'>
            <Label>{t('Channel ID')}</Label>
            <Input
              inputMode='numeric'
              value={draft.channelId}
              onChange={(e) =>
                setDraft({ ...draft, channelId: e.target.value })
              }
              placeholder='49'
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Model (empty = channel default)')}</Label>
            <Input
              value={draft.model}
              onChange={(e) => setDraft({ ...draft, model: e.target.value })}
              placeholder='deepseek-v4.1-flash'
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Billing Mode')}</Label>
            <NativeSelect
              value={draft.mode}
              onChange={(e) =>
                setDraft({ ...draft, mode: e.target.value as Draft['mode'] })
              }
            >
              <NativeSelectOption value='call'>
                {t('Per call')}
              </NativeSelectOption>
              <NativeSelectOption value='token'>
                {t('Per token')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Cost per call (CNY)')}</Label>
            <Input
              inputMode='decimal'
              value={draft.perCall}
              onChange={(e) => setDraft({ ...draft, perCall: e.target.value })}
              placeholder='0.0007'
              disabled={draft.mode !== 'call'}
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Input cost per 1M tokens (CNY)')}</Label>
            <Input
              inputMode='decimal'
              value={draft.inputPerMillion}
              onChange={(e) =>
                setDraft({ ...draft, inputPerMillion: e.target.value })
              }
              placeholder='0'
              disabled={draft.mode !== 'token'}
            />
          </div>
          <div className='space-y-1.5'>
            <Label>{t('Output cost per 1M tokens (CNY)')}</Label>
            <Input
              inputMode='decimal'
              value={draft.outputPerMillion}
              onChange={(e) =>
                setDraft({ ...draft, outputPerMillion: e.target.value })
              }
              placeholder='0'
              disabled={draft.mode !== 'token'}
            />
          </div>
          <div className='space-y-1.5 sm:col-span-2'>
            <Label>{t('Note (optional)')}</Label>
            <Input
              value={draft.note}
              onChange={(e) => setDraft({ ...draft, note: e.target.value })}
              placeholder={t('Price adjustment')}
            />
          </div>
        </div>
      </Dialog>
    </div>
  )
}
