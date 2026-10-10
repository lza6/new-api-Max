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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RotateCcw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { RiskAcknowledgementDialog } from '@/components/risk-acknowledgement-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getFeatureSwitches, updateFeatureSwitch } from '../api'
import type {
  FeatureSwitchSnapshot,
  FeatureSwitchUpdateRequest,
} from '../types'

const FEATURE_SWITCHES_QUERY_KEY = ['feature-switches'] as const

type PendingChange = {
  snapshot: FeatureSwitchSnapshot
  value: string
}

function riskVariant(risk: FeatureSwitchSnapshot['risk']) {
  switch (risk) {
    case 'high':
      return 'destructive' as const
    case 'medium':
      return 'secondary' as const
    default:
      return 'outline' as const
  }
}

function riskLabelKey(risk: FeatureSwitchSnapshot['risk']) {
  switch (risk) {
    case 'high':
      return 'High risk'
    case 'medium':
      return 'Medium risk'
    default:
      return 'Low risk'
  }
}

function formatMetricValue(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

/**
 * 能力开关管理列表。
 *
 * 设计要点（对应改进指南 G1 的 UI 要求）：
 * 1. 每个开关显示当前值 / 默认值 / 风险 / 依赖 / 是否需重启 / 回滚提示；
 * 2. **效果度量是这一页存在的主要理由**：开关关闭时明确显示「尚无数据」，
 *    开启后显示与该开关相关的实际计数，让管理员看得见打开之后发生了什么；
 * 3. 高风险开关必须二次确认（复用 `RiskAcknowledgementDialog`，要求逐字
 *    输入开关名），避免一键把站点打成不可用。
 */
export function FeatureSwitchList() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pendingChange, setPendingChange] = useState<PendingChange | null>(null)

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: FEATURE_SWITCHES_QUERY_KEY,
    queryFn: getFeatureSwitches,
  })

  const mutation = useMutation({
    mutationFn: async (request: FeatureSwitchUpdateRequest) =>
      requireServerSuccess(await updateFeatureSwitch(request)),
    onSuccess: (response) => {
      queryClient.setQueryData(FEATURE_SWITCHES_QUERY_KEY, response)
      toast.success(t('Feature switch updated'))
    },
    onError: (error: Error) => {
      handleServerError(error, t('Failed to update feature switch'))
    },
  })

  const switches = useMemo(() => data?.data?.switches ?? [], [data])
  const metrics = data?.data?.metrics ?? {}

  if (isLoading) {
    return <LoadingState message={t('Loading settings...')} />
  }
  if (isError || !data?.data) {
    return (
      <ErrorState
        title={t('Failed to load feature switches')}
        onRetry={() => {
          void refetch()
        }}
      />
    )
  }

  const applyChange = (request: FeatureSwitchUpdateRequest) => {
    setPendingChange(null)
    mutation.mutate(request)
  }

  const requestToggle = (snapshot: FeatureSwitchSnapshot, value: string) => {
    // 高风险开关一律先走二次确认，不做「一键生效」。
    if (snapshot.risk === 'high') {
      setPendingChange({ snapshot, value })
      return
    }
    applyChange({ key: snapshot.key, value })
  }

  return (
    <div className='flex flex-col gap-4'>
      <p className='text-muted-foreground text-sm'>
        {t(
          'These capabilities are already implemented but disabled by default. Turn one on to start collecting data, then check its effect metrics before depending on it.'
        )}
      </p>

      {switches.map((snapshot) => (
        <FeatureSwitchCard
          key={snapshot.key}
          snapshot={snapshot}
          metrics={metrics}
          isMutating={mutation.isPending}
          onToggle={requestToggle}
          onReset={(key) => applyChange({ key, reset: true })}
        />
      ))}

      <RiskAcknowledgementDialog
        open={pendingChange !== null}
        onOpenChange={(open) => {
          if (!open) setPendingChange(null)
        }}
        title={t('Enable high-risk capability')}
        description={
          pendingChange
            ? t(pendingChange.snapshot.description_key)
            : undefined
        }
        items={
          pendingChange
            ? [
                `${t('Switch')}: ${pendingChange.snapshot.key}`,
                `${t('Rollback')}: ${t(pendingChange.snapshot.rollback_hint)}`,
              ]
            : []
        }
        inputPrompt={t('Type the switch name to confirm')}
        requiredText={pendingChange?.snapshot.key}
        inputPlaceholder={pendingChange?.snapshot.key}
        destructive
        isLoading={mutation.isPending}
        onConfirm={() => {
          if (pendingChange) {
            applyChange({ key: pendingChange.snapshot.key, value: pendingChange.value })
          }
        }}
      />
    </div>
  )
}

type FeatureSwitchCardProps = {
  snapshot: FeatureSwitchSnapshot
  metrics: Record<string, number>
  isMutating: boolean
  onToggle: (snapshot: FeatureSwitchSnapshot, value: string) => void
  onReset: (key: string) => void
}

function FeatureSwitchCard({
  snapshot,
  metrics,
  isMutating,
  onToggle,
  onReset,
}: FeatureSwitchCardProps) {
  const { t } = useTranslation()
  const enabled = snapshot.kind === 'enum'
    ? snapshot.value !== 'off'
    : snapshot.value === 'true'
  const metricKeys = snapshot.metric_keys ?? []
  const hasMetricData = metricKeys.some((key) => key in metrics)

  return (
    <Card>
      <CardHeader className='gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <CardTitle className='text-base'>{t(snapshot.title_key)}</CardTitle>
          <Badge variant={riskVariant(snapshot.risk)}>
            {t(riskLabelKey(snapshot.risk))}
          </Badge>
          {snapshot.requires_restart ? (
            <Badge variant='outline'>{t('Requires restart')}</Badge>
          ) : null}
          {!snapshot.admin_editable ? (
            <Badge variant='outline'>
              {t('Read-only: change with an environment variable')}
            </Badge>
          ) : null}
          {snapshot.unsatisfied_deps?.length ? (
            <Badge variant='destructive'>
              {t('Blocked by a disabled dependency')}
            </Badge>
          ) : null}
        </div>
        <CardDescription>{t(snapshot.description_key)}</CardDescription>
      </CardHeader>

      <CardContent className='flex flex-col gap-4'>
        <div className='flex flex-wrap items-center justify-between gap-4'>
          <div className='text-muted-foreground flex flex-col gap-1 text-xs'>
            <span>
              {t('Current value')}:{' '}
              <span className='text-foreground font-medium'>
                {snapshot.value}
              </span>
            </span>
            <span>
              {t('Environment default')}:{' '}
              <span className='text-foreground font-medium'>
                {snapshot.env_default}
              </span>
            </span>
            <span>
              {snapshot.configured
                ? t('Configured in the admin console')
                : t('Using the environment default')}
            </span>
            <span>
              {t('Switch')}:{' '}
              <span className='text-foreground font-mono'>{snapshot.key}</span>
            </span>
          </div>

          <div className='flex items-center gap-3'>
            {snapshot.configured && snapshot.admin_editable ? (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                disabled={isMutating}
                onClick={() => onReset(snapshot.key)}
              >
                <RotateCcw className='size-4' />
                {t('Reset to default')}
              </Button>
            ) : null}
            {snapshot.admin_editable ? (
              snapshot.kind === 'enum' ? (
                <Select
                  value={snapshot.value}
                  disabled={isMutating}
                  onValueChange={(value) => {
                    // Base UI 的 Select 在清空时回调 null；忽略空值，避免发出非法变更。
                    if (value === null) return
                    onToggle(snapshot, value)
                  }}
                >
                  <SelectTrigger
                    className='w-36'
                    aria-label={t(snapshot.title_key)}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(snapshot.options ?? []).map((option) => (
                      <SelectItem key={option} value={option}>
                        {option}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Switch
                  checked={enabled}
                  disabled={isMutating}
                  aria-label={t(snapshot.title_key)}
                  onCheckedChange={(checked) =>
                    onToggle(snapshot, checked ? 'true' : 'false')
                  }
                />
              )
            ) : null}
          </div>
        </div>

        <Separator />

        <div className='flex flex-col gap-2'>
          <span className='text-sm font-medium'>{t('Effect metrics')}</span>
          {hasMetricData ? (
            <ul className='text-muted-foreground flex flex-wrap gap-x-6 gap-y-1 text-xs'>
              {metricKeys.map((key) => (
                <li key={key}>
                  <span className='text-foreground font-mono'>{key}</span>
                  {' = '}
                  {key in metrics
                    ? formatMetricValue(metrics[key])
                    : t('no data yet')}
                </li>
              ))}
            </ul>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {enabled
                ? t('No metrics are wired for this switch yet.')
                : t('Not enabled, so there is no data yet.')}
            </p>
          )}
        </div>

        <p className='text-muted-foreground text-xs'>
          {t('Rollback')}: {t(snapshot.rollback_hint)}
        </p>
      </CardContent>
    </Card>
  )
}
