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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDown,
  ArrowUp,
  Layers2,
  Plus,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@/components/ui/empty'
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { getChannels } from '../api'
import { formatRelativeTime } from '../lib'
import {
  comboQueryKeys,
  parseComboModels,
  stringifyComboModels,
  getCombos,
  createCombo,
  updateCombo,
  deleteCombo,
  type ChannelCombo,
  type ComboPayload,
  type ComboStrategy,
} from '../lib/combo-api'
import type { Channel } from '../types'

// ============================================================================
// Constants
// ============================================================================

const COMBO_STRATEGIES: Array<{ value: ComboStrategy; label: string }> = [
  { value: 'fallback', label: 'Sequential fallback' },
  { value: 'round-robin', label: 'Round-robin (sticky)' },
  { value: 'weighted', label: 'Weighted random' },
]

const COMBO_STRATEGY_LABEL: Record<ComboStrategy, string> = {
  fallback: 'Sequential fallback',
  'round-robin': 'Round-robin (sticky)',
  weighted: 'Weighted random',
}

const DEFAULT_STICKY = 1

// ============================================================================
// Form schema
// ============================================================================

const comboFormSchema = z.object({
  name: z.string().min(1).max(64),
  strategy: z.enum(['fallback', 'round-robin', 'weighted']),
  sticky: z.number().int().min(1).max(1000),
  status: z.number(),
})

type ComboFormValues = z.infer<typeof comboFormSchema>

const EMPTY_FORM_VALUES: ComboFormValues = {
  name: '',
  strategy: 'fallback',
  sticky: DEFAULT_STICKY,
  status: 1,
}

// ============================================================================
// Helpers
// ============================================================================

type CandidateRow = {
  id: number
  channel_id: number
  model: string
  weight?: number
}

function createEmptyRow(): CandidateRow {
  return { id: nextRowId++, channel_id: 0, model: '', weight: undefined }
}

let nextRowId = 0

function formatStickyLabel(strategy: ComboStrategy): string {
  return strategy === 'round-robin' ? 'Sticky requests' : ''
}

// ============================================================================
// List table
// ============================================================================

type ComboListTableProps = {
  combos: ChannelCombo[]
  onEdit: (combo: ChannelCombo) => void
  onDelete: (combo: ChannelCombo) => void
}

function ComboListTable({ combos, onEdit, onDelete }: ComboListTableProps) {
  const { t } = useTranslation()
  const i18n = useTranslation().i18n
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const columns = useMemo<StaticDataTableColumn<ChannelCombo>[]>(
    () => [
      {
        id: 'name',
        header: t('Name'),
        cell: (combo) => (
          <button
            type='button'
            className='text-primary hover:underline'
            onClick={() => onEdit(combo)}
          >
            {combo.name}
          </button>
        ),
      },
      {
        id: 'strategy',
        header: t('Strategy'),
        cell: (combo) => (
          <Badge variant='outline'>{t(COMBO_STRATEGY_LABEL[combo.strategy])}</Badge>
        ),
      },
      {
        id: 'candidates',
        header: t('Candidates'),
        cell: (combo) => `${parseComboModels(combo.models).length}`,
      },
      {
        id: 'sticky',
        header: t('Sticky'),
        cell: (combo) =>
          combo.strategy === 'round-robin' ? String(combo.sticky) : '-',
      },
      {
        id: 'status',
        header: t('Status'),
        cell: (combo) =>
          combo.status === 1 ? (
            <StatusBadge label={t('Enabled')} variant='success' copyable={false} />
          ) : (
            <StatusBadge label={t('Disabled')} variant='neutral' copyable={false} />
          ),
      },
      {
        id: 'updated',
        header: t('Updated'),
        cell: (combo) =>
          combo.updated ? formatRelativeTime(combo.updated, locale) : '-',
      },
      {
        id: 'actions',
        header: '',
        className: 'w-[120px]',
        cell: (combo) => (
          <div className='flex items-center justify-end gap-1'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onEdit(combo)}
            >
              {t('Edit')}
            </Button>
            <Button
              type='button'
              variant='destructive'
              size='sm'
              onClick={() => onDelete(combo)}
            >
              <Trash2 className='h-4 w-4' aria-hidden='true' />
              <span className='sr-only'>{t('Delete')}</span>
            </Button>
          </div>
        ),
      },
    ],
    [t, onEdit, onDelete, locale]
  )

  return (
    <StaticDataTable<ChannelCombo>
      columns={columns}
      data={combos}
      getRowKey={(combo) => combo.id}
      emptyContent={
        <Empty>
          <EmptyHeader>
            <EmptyTitle>{t('No Model Combos')}</EmptyTitle>
            <EmptyDescription>
              {t('No model combos yet. Click "New Combo" to create one.')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      }
    />
  )
}

// ============================================================================
// Candidate rows editor
// ============================================================================

type CandidateRowsEditorProps = {
  channels: Channel[]
  items: CandidateRow[]
  strategy: ComboStrategy
  onChange: (items: CandidateRow[]) => void
}

function CandidateRowsEditor({
  channels,
  items,
  strategy,
  onChange,
}: CandidateRowsEditorProps) {
  const { t } = useTranslation()

  const channelOptions = useMemo(
    () =>
      channels.map((channel) => ({
        value: channel.id,
        label: `#${channel.id} · ${channel.name}`,
      })),
    [channels]
  )

  const channelNameById = useMemo(
    () => new Map(channels.map((channel) => [channel.id, channel.name])),
    [channels]
  )

  const updateRow = (index: number, patch: Partial<CandidateRow>) => {
    const next = items.map((item, i) => (i === index ? { ...item, ...patch } : item))
    onChange(next)
  }

  const removeRow = (index: number) => {
    onChange(items.filter((_, i) => i !== index))
  }

  const moveRow = (index: number, delta: -1 | 1) => {
    const target = index + delta
    if (target < 0 || target >= items.length) {return}
    const next = [...items]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    onChange(next)
  }

  const addRow = () => {
    onChange([...items, createEmptyRow()])
  }

  return (
    <div className='space-y-2'>
      {items.length > 0 ? (
        <div className='space-y-2'>
          {items.map((item, index) => {
            const channelName = item.channel_id ? channelNameById.get(item.channel_id) : undefined
            return (
              <div
                key={item.id}
                className='border-border/60 rounded-lg border p-2'
              >
                <div className='grid grid-cols-[1fr_auto] items-center gap-2 sm:grid-cols-[1fr_1fr_auto_auto]'>
                  <Select
                    value={String(item.channel_id || '')}
                    onValueChange={(value) =>
                      updateRow(index, {
                        channel_id: value ? Number(value) : 0,
                      })
                    }
                  >
                    <SelectTrigger
                      className='w-full'
                      aria-label={t('Channel')}
                    >
                      <SelectValue placeholder={t('Select channel')} />
                    </SelectTrigger>
                    <SelectContent>
                      {channelOptions.map((option) => (
                        <SelectItem key={option.value} value={String(option.value)}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>

                  <Input
                    value={item.model}
                    onChange={(e) => updateRow(index, { model: e.target.value })}
                    placeholder={t('Model name')}
                    aria-label={t('Model name')}
                    className='sm:col-span-1'
                  />

                  <div className='col-span-2 flex items-center gap-1 sm:col-span-1 sm:justify-end'>
                    {strategy === 'weighted' && (
                      <Input
                        type='number'
                        min={1}
                        value={item.weight ?? 1}
                        onChange={(e) =>
                          updateRow(index, {
                            weight: e.target.value ? Number(e.target.value) : undefined,
                          })
                        }
                        className='w-20'
                        aria-label={t('Weight')}
                      />
                    )}
                    <Tooltip>
                      <TooltipTrigger render={<span className='inline-flex' />}>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          onClick={() => moveRow(index, -1)}
                          disabled={index === 0}
                          aria-label={t('Move up')}
                        >
                          <ArrowUp className='h-4 w-4' aria-hidden='true' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t('Move up')}</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger render={<span className='inline-flex' />}>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          onClick={() => moveRow(index, 1)}
                          disabled={index === items.length - 1}
                          aria-label={t('Move down')}
                        >
                          <ArrowDown className='h-4 w-4' aria-hidden='true' />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{t('Move down')}</TooltipContent>
                    </Tooltip>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-sm'
                      onClick={() => removeRow(index)}
                      aria-label={t('Remove candidate')}
                      className='text-destructive hover:text-destructive'
                    >
                      <Trash2 className='h-4 w-4' aria-hidden='true' />
                    </Button>
                  </div>
                </div>
                <div className='text-muted-foreground mt-1 pl-1 text-xs'>
                  {channelName
                    ? `${t('Channel')}: ${channelName}`
                    : t(
                        item.channel_id
                          ? 'Channel #{{id}}'
                          : 'Select a channel',
                        item.channel_id ? { id: item.channel_id } : undefined
                      )}
                </div>
              </div>
            )
          })}
        </div>
      ) : (
        <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed text-sm'>
          {t('No candidates yet. Click "Add Candidate" to add one.')}
        </div>
      )}

      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-full'
        onClick={addRow}
      >
        <Plus className='mr-2 h-4 w-4' />
        {t('Add Candidate')}
      </Button>
    </div>
  )
}

// ============================================================================
// Editor form
// ============================================================================

type ComboEditorProps = {
  editing: ChannelCombo | null
  channels: Channel[]
  channelsLoading: boolean
  onSaved: () => void
}

function ComboEditor({
  editing,
  channels,
  channelsLoading,
  onSaved,
}: ComboEditorProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [candidates, setCandidates] = useState<CandidateRow[]>([])

  const form = useForm<ComboFormValues>({
    resolver: zodResolver(comboFormSchema),
    defaultValues: EMPTY_FORM_VALUES,
  })

  const watchedStrategy = form.watch('strategy')
  const watchedStatus = form.watch('status')

  // Initialize the form when the editing target changes.
  useEffect(() => {
    if (editing) {
      form.reset({
        name: editing.name,
        strategy: editing.strategy,
        sticky: editing.sticky || DEFAULT_STICKY,
        status: editing.status,
      })
      setCandidates(parseComboModels(editing.models).map((item) => ({ id: nextRowId++, ...item })))
    } else {
      form.reset(EMPTY_FORM_VALUES)
      setCandidates([])
    }
  }, [editing, form])

  const { mutate, isPending: isSubmitting } = useMutation({
    mutationFn: async (payload: ComboPayload): Promise<void> => {
      if (editing) {
        await updateCombo({ ...payload, id: editing.id })
      } else {
        await createCombo(payload)
      }
    },
    onSuccess: () => {
      toast.success(t(editing ? 'Combo updated successfully' : 'Combo created successfully'))
      queryClient.invalidateQueries({ queryKey: comboQueryKeys.lists() })
      onSaved()
    },
    onError: (error: unknown) => {
      handleServerError(
        error,
        t(editing ? 'Failed to update combo' : 'Failed to create combo')
      )
    },
  })

  const handleCancel = () => {
    if (isSubmitting) {return}
    onSaved()
  }

  const onSubmit = (values: ComboFormValues) => {
    const payload: ComboPayload = {
      ...values,
      models: stringifyComboModels(candidates),
    }

    // Local guard: at least one fully selected candidate.
    if (
      candidates.length === 0 ||
      candidates.some((c) => !c.channel_id || !c.model?.trim())
    ) {
      toast.error(t('Each combo candidate must select a channel and model'))
      return
    }

    mutate(payload)
  }

  return (
    <Form {...form}>
      <form
        id='combo-editor-form'
        onSubmit={form.handleSubmit(onSubmit)}
        className={sideDrawerFormClassName()}
      >
        <div className='grid gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Combo name')}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='strategy'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Strategy')}</FormLabel>
                <FormControl>
                  <Select
                    value={field.value}
                    onValueChange={(value) => field.onChange(value as ComboStrategy)}
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue placeholder={t('Select strategy')} />
                    </SelectTrigger>
                    <SelectContent>
                      {COMBO_STRATEGIES.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {t(option.label)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {watchedStrategy === 'round-robin' && (
            <FormField
              control={form.control}
              name='sticky'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Sticky requests')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      value={field.value}
                      onChange={(e) => field.onChange(Number(e.target.value))}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <FormField
            control={form.control}
            name='status'
            render={({ field }) => (
              <FormItem className='border-border/60 flex min-h-16 flex-row items-center justify-between gap-3 rounded-lg border px-3 py-2'>
                <div className='space-y-0.5'>
                  <FormLabel>{t('Enabled')}</FormLabel>
                  <p className='text-muted-foreground text-xs'>
                    {t('Disabled combos are ignored during routing')}
                  </p>
                </div>
                <FormControl>
                  <Switch
                    checked={watchedStatus === 1}
                    onCheckedChange={(checked) => field.onChange(checked ? 1 : 2)}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </div>

        <Separator />

        <div className='space-y-3'>
          <div className='flex items-center justify-between'>
            <Label className='text-sm'>{t('Candidates')}</Label>
            <span className='text-muted-foreground text-xs'>
              {t(formatStickyLabel(watchedStrategy))}
            </span>
          </div>

          {channelsLoading ? (
            <div className='space-y-2'>
              <Skeleton className='h-24 w-full' />
              <Skeleton className='h-24 w-full' />
            </div>
          ) : (
            <CandidateRowsEditor
              channels={channels}
              items={candidates}
              strategy={watchedStrategy}
              onChange={setCandidates}
            />
          )}

          {channels.length === 0 && !channelsLoading && (
            <p className='text-muted-foreground text-xs'>
              {t('No channels available. Create a channel first.')}
            </p>
          )}
        </div>
      </form>

      <div className='border-border/70 bg-background/95 grid grid-cols-2 gap-2 border-t px-4 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/80 sm:flex sm:flex-row sm:justify-end sm:px-6 sm:py-4'>
        <Button
          type='button'
          variant='outline'
          onClick={handleCancel}
          disabled={isSubmitting}
        >
          {t('Cancel')}
        </Button>
        <Button
          type='submit'
          form='combo-editor-form'
          disabled={isSubmitting}
        >
          {isSubmitting && (
            <span className='mr-1 inline-block size-3 animate-spin rounded-full border-2 border-current border-t-transparent' />
          )}
          {t('Save')}
        </Button>
      </div>
    </Form>
  )
}

// ============================================================================
// Drawer
// ============================================================================

type ComboManagerDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ComboManagerDrawer({
  open,
  onOpenChange,
}: ComboManagerDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<ChannelCombo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ChannelCombo | null>(null)
  const [deleteLoading, setDeleteLoading] = useState(false)

  const { data: combos, isLoading: combosLoading } = useQuery({
    queryKey: comboQueryKeys.lists(),
    queryFn: async () => requireServerSuccess(await getCombos()),
    enabled: open,
  })

  const { data: channels, isLoading: channelsLoading } = useQuery({
    queryKey: ['channel-combo-candidates'],
    queryFn: async () => requireServerSuccess(await getChannels({ page_size: 100 })),
    enabled: open,
  })

  const comboList = useMemo(() => combos ?? [], [combos])
  const channelList = useMemo(() => channels?.data?.items ?? [], [channels])

  let body: React.ReactNode
  if (editing) {
    body = (
      <ComboEditor
        editing={editing}
        channels={channelList}
        channelsLoading={channelsLoading}
        onSaved={() => setEditing(null)}
      />
    )
  } else if (combosLoading) {
    body = (
      <div className='space-y-2'>
        <Skeleton className='h-12 w-full' />
        <Skeleton className='h-12 w-full' />
        <Skeleton className='h-12 w-full' />
      </div>
    )
  } else {
    body = (
      <div className='flex flex-col gap-3'>
        <div className='flex items-center justify-between'>
          <div className='text-muted-foreground text-xs'>
            {t('{{count}} combos', { count: comboList.length })}
          </div>
          <Button
            type='button'
            size='sm'
            onClick={() => setEditing({} as ChannelCombo)}
          >
            <Plus className='h-4 w-4' />
            {t('New Combo')}
          </Button>
        </div>

        {comboList.length > 0 ? (
          <ComboListTable
            combos={comboList}
            onEdit={(combo) => setEditing(combo)}
            onDelete={(combo) => setDeleteTarget(combo)}
          />
        ) : (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>{t('No Model Combos')}</EmptyTitle>
              <EmptyDescription>
                {t('No model combos yet. Click "New Combo" to create one.')}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>
    )
  }

  // Reset transient state when the drawer closes.
  useEffect(() => {
    if (!open) {
      setEditing(null)
      setDeleteTarget(null)
      setDeleteLoading(false)
    }
  }, [open])

  const handleClose = () => {
    if (!open) {return}
    onOpenChange(false)
    setEditing(null)
    setDeleteTarget(null)
  }

  const handleDeleteConfirm = async () => {
    if (!deleteTarget) {return}
    setDeleteLoading(true)
    try {
      await deleteCombo(deleteTarget.id)
      toast.success(t('Combo deleted successfully'))
      queryClient.invalidateQueries({ queryKey: comboQueryKeys.lists() })
      setDeleteTarget(null)
    } catch (error) {
      handleServerError(error, t('Failed to delete combo'))
    } finally {
      setDeleteLoading(false)
    }
  }

  return (
    <>
      <Sheet open={open} onOpenChange={handleClose}>
        <SheetContent
          className={cn(sideDrawerContentClassName('sm:max-w-4xl'))}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle className='flex items-center gap-2'>
              <Layers2 className='h-5 w-5' aria-hidden='true' />
              {t('Model Combos')}
            </SheetTitle>
            <SheetDescription>
              {t(
                'A model combo maps a model name to multiple channel/model candidates with fallback, round-robin, or weighted routing.'
              )}
            </SheetDescription>
          </SheetHeader>

          <div
            className={cn(
              sideDrawerFormClassName(),
              'justify-between'
            )}
          >
            {body}
          </div>

          {!editing && (
            <SheetFooter className={sideDrawerFooterClassName()}>
              <Button type='button' variant='outline' onClick={handleClose}>
                {t('Close')}
              </Button>
            </SheetFooter>
          )}
        </SheetContent>
      </Sheet>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(v) => !v && setDeleteTarget(null)}
        title={t('Delete this model combo?')}
        desc={t(
          'This will permanently delete the model combo "{{name}}". This action cannot be undone.',
          { name: deleteTarget?.name ?? '' }
        )}
        destructive
        confirmText={t('Delete')}
        isLoading={deleteLoading}
        handleConfirm={handleDeleteConfirm}
      />
    </>
  )
}
