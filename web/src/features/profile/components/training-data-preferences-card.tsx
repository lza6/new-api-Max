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
import { Loader2, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { RiskAcknowledgementDialog } from '@/components/risk-acknowledgement-dialog'
import { Switch } from '@/components/ui/switch'
import { TitledCard } from '@/components/ui/titled-card'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import {
  getTrainingDataStatus,
  grantTrainingDataConsent,
  revokeTrainingDataConsent,
} from '../api'

/**
 * React Query 缓存键（与其它用户级状态一致，按用户维度隔离）。
 *
 * 不导出：导出非组件符号会破坏 Fast Refresh（oxlint react/only-export-components）。
 * 目前只有本组件使用，需要共享时再抽到同 feature 的 constants 文件。
 */
const trainingDataStatusQueryKey = ['training-data', 'status'] as const

/**
 * 训练数据采集授权卡片（Spec-008）。
 *
 * ## 交互契约（每条都来自明确要求）
 *
 * 1. **不可用时不渲染**：管理员关闭总开关、或云盘未配置时后端下发
 *    `available: false` —— 此时本组件返回 null。给用户一个「能点但没作用」
 *    的开关是误导，用户明确要求过「管理员关了用户侧就不该有这个开关」。
 * 2. **开启必须经免责声明确认**：点开关**不直接生效**，而是弹出声明，
 *    用户勾选确认后才调用授权接口。
 * 3. **常驻小字说明**：卡片内始终显示一段说明，包括「撤回只停止后续采集、
 *    不删除已上传数据」这一**真实语义边界** —— 不写成模糊的「随时可删除」，
 *    那是做不到的承诺。
 * 4. **实时状态**：授权/撤回后以后端返回值为准更新（不信本地推测）。
 */
export function TrainingDataPreferencesCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [disclaimerOpen, setDisclaimerOpen] = useState(false)
  const [revokeOpen, setRevokeOpen] = useState(false)

  const statusQuery = useQuery({
    queryKey: trainingDataStatusQueryKey,
    queryFn: async () => {
      const response = await getTrainingDataStatus()
      if (!response.success) {
        throw createServerError(response, t('Failed to load settings'))
      }
      return response.data
    },
  })

  const grantMutation = useMutation({
    mutationFn: async (policyVersion: string) => {
      const response = await grantTrainingDataConsent(policyVersion)
      if (!response.success) {
        throw createServerError(response, t('Failed to update settings'))
      }
      return response.data
    },
    onSuccess: (data) => {
      queryClient.setQueryData(trainingDataStatusQueryKey, data)
      toast.success(t('Training data collection enabled'))
      setDisclaimerOpen(false)
    },
    onError: (error) => {
      handleServerError(error, t('Failed to update settings'))
    },
  })

  const revokeMutation = useMutation({
    mutationFn: async () => {
      const response = await revokeTrainingDataConsent()
      if (!response.success) {
        throw createServerError(response, t('Failed to update settings'))
      }
      return response.data
    },
    onSuccess: (data) => {
      queryClient.setQueryData(trainingDataStatusQueryKey, data)
      toast.success(t('Training data collection disabled'))
      setRevokeOpen(false)
    },
    onError: (error) => {
      handleServerError(error, t('Failed to update settings'))
    },
  })

  const status = statusQuery.data

  // 不可用（功能未开或云盘未配置）→ 整张卡片不出现。
  if (!status?.available) {
    return null
  }

  const busy =
    statusQuery.isLoading || grantMutation.isPending || revokeMutation.isPending
  const granted = status.granted

  const handleCheckedChange = (next: boolean) => {
    if (next) {
      // 开启走免责声明确认，不直接改状态。
      setDisclaimerOpen(true)
      return
    }
    setRevokeOpen(true)
  }

  return (
    <>
      <TitledCard
        title={t('Training Data Collection')}
        description={t('Help improve model quality with your conversations')}
        icon={<ShieldCheck className='h-4 w-4' />}
        iconTone='chart-2'
        disableHoverEffect
      >
        <div className='space-y-3'>
          <div className='flex items-start justify-between gap-3'>
            <div className='min-w-0'>
              <p className='text-sm font-medium'>
                {granted
                  ? t('Collection is on')
                  : t('Collection is off')}
              </p>
              {granted && status.granted_at > 0 && (
                <p className='text-muted-foreground text-xs'>
                  {t('Authorized on {{date}}', {
                    date: new Date(status.granted_at * 1000).toLocaleString(),
                  })}
                </p>
              )}
              {/*
                声明改版 → 旧同意失效。开关会从「开」变成「关」，
                必须说明**为什么**，否则用户会以为功能坏了、自己误触了，
                甚至怀疑数据在被偷偷继续采集。 */}
              {status.needs_reconsent && (
                <p className='text-xs text-amber-600 dark:text-amber-500'>
                  {t(
                    'The collection notice was updated. Collection is paused until you review and agree to the new notice.'
                  )}
                </p>
              )}
            </div>
            <div className='flex shrink-0 items-center gap-2'>
              {busy && (
                <Loader2 className='text-muted-foreground size-3.5 animate-spin' />
              )}
              <Switch
                checked={granted}
                disabled={busy}
                onCheckedChange={handleCheckedChange}
                aria-label={t('Training Data Collection')}
              />
            </div>
          </div>

          {/*
            常驻小字：必须如实说明三件事 —— 采什么、去哪、撤回的边界。
            最后一条尤其重要：撤回**不删除已上传数据**，写成模糊承诺
            会让用户产生错误预期。 */}
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t(
              'When enabled, successful requests are uploaded asynchronously to the administrator-configured cloud storage for model training. Turning it off stops future collection immediately, but does not delete data already uploaded.'
            )}
          </p>

          {status.reason && (
            <p className='text-muted-foreground text-xs'>{status.reason}</p>
          )}
        </div>
      </TitledCard>

      {/*
        免责声明：必须勾选确认才能开启。
        用项目既有的 RiskAcknowledgementDialog（复用其 checklist 与
        "必须勾选"的交互），而不是在 feature 内另拼一套弹窗。 */}
      <RiskAcknowledgementDialog
        open={disclaimerOpen}
        onOpenChange={setDisclaimerOpen}
        title={t('Training Data Collection Disclaimer')}
        description={t(
          'Please read carefully before enabling training data collection.'
        )}
        items={[
          t(
            'Only successfully completed requests are collected. Failed requests are never collected.'
          ),
          t(
            'Collected content is uploaded to cloud storage configured by the site administrator, and is used to improve model quality.'
          ),
          // 敏感面必须讲全：只讲「采你的输入」会让人以为回复不算内容。
          // 回复常复述用户贴过的密钥/凭据，且管理员可读可导出原始对话 ——
          // 这两点不写清楚，用户的「同意」就不算知情同意。
          t(
            'Both your messages and the model replies are stored. Site administrators can read and export the raw conversation, so never send secrets, credentials or personal data you are not willing to have reviewed.'
          ),
          t(
            'Your conversations are also sent to the upstream AI models you use, since that is how you get a reply. Additionally, the site administrator may send a collected conversation to a model for quality analysis.'
          ),
          t(
            'Collection never blocks or slows down your requests: it runs asynchronously with a bandwidth limit of about 10% of the server uplink.'
          ),
          t(
            'You may turn it off at any time. Collection stops immediately, but data already uploaded is not deleted automatically.'
          ),
        ]}
        checklist={[
          t('I have read and agree to the above disclaimer'),
        ]}
        confirmText={t('Agree and enable')}
        cancelText={t('Cancel')}
        destructive={false}
        isLoading={grantMutation.isPending}
        onConfirm={() => grantMutation.mutate(status.policy_version)}
      />

      <ConfirmDialog
        open={revokeOpen}
        onOpenChange={setRevokeOpen}
        title={t('Turn off training data collection')}
        desc={t(
          'Collection stops immediately. Data already uploaded is not deleted.'
        )}
        destructive
        isLoading={revokeMutation.isPending}
        confirmText={t('Turn off')}
        handleConfirm={() => revokeMutation.mutate()}
      />
    </>
  )
}
