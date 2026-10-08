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
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  PromptInput,
  PromptInputAttachment,
  PromptInputAttachments,
  PromptInputFooter,
  PromptInputTextarea,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'

import { blobUrlToDataUrl, extractInputImageUrls, getSubmittableInputText } from '../../lib'
import {
  getEstimateGroups,
  getEstimatePlan,
  type EstimateGroup,
  type EstimatePlan,
} from '../../lib/cost-estimate'
import type {
  ModelOption,
  GroupOption,
  ParameterEnabled,
  PlaygroundConfig,
} from '../../types'
import { PlaygroundInputControls } from './playground-input-controls'
import { PlaygroundInputTools } from './playground-input-tools'
import { CostEstimateHint } from './cost-estimate-hint'

interface PlaygroundInputProps {
  config: PlaygroundConfig
  onSubmit: (text: string, imageUrls?: string[]) => void
  onStop?: () => void
  disabled?: boolean
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  groups: GroupOption[]
  groupValue: string
  onGroupChange: (value: string) => void
  hasMessages?: boolean
  onConfigChange: <K extends keyof PlaygroundConfig>(
    key: K,
    value: PlaygroundConfig[K]
  ) => void
  onClearMessages?: () => void
  onParameterEnabledChange: (
    key: keyof ParameterEnabled,
    value: boolean
  ) => void
  parameterEnabled: ParameterEnabled
  estimateModel?: {
    model_name: string
    enable_groups: string[]
    group_ratio?: Record<string, number>
    billing_expr?: string
    billing_usage_schema?: Record<
      string,
      { type?: 'number' | 'boolean'; unit?: string }
    >
    billing_usage_examples?: {
      label: string
      facts: Record<string, number | string>
    }[]
  } | null
}

export function PlaygroundInput({
  config,
  onSubmit,
  onStop,
  disabled,
  isGenerating,
  models,
  modelValue,
  onModelChange,
  isModelLoading = false,
  groups,
  groupValue,
  onGroupChange,
  hasMessages = false,
  onConfigChange,
  onClearMessages,
  onParameterEnabledChange,
  parameterEnabled,
  estimateModel,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [isPreparingImages, setIsPreparingImages] = useState(false)

  const handleSubmit = async (message: PromptInputMessage) => {
    const submittableText = getSubmittableInputText(message, disabled)

    if (submittableText === null) {return}
    // 附件是 blob: URL（浏览器本地对象），上游无法获取 → 转成 data: URL 再发。
    const rawUrls = extractInputImageUrls(message)
    let imageUrls: string[] = []
    if (rawUrls.length > 0) {
      setIsPreparingImages(true)
      try {
        imageUrls = await Promise.all(rawUrls.map(blobUrlToDataUrl))
      } catch {
        // 转换失败：跳过图片而非阻断发送（纯文本仍可发）。
        imageUrls = []
      } finally {
        setIsPreparingImages(false)
      }
    }
    // 防「空提交」：无文字且图片全部转换失败时，不发送空消息。
    if (!submittableText.trim() && imageUrls.length === 0) {
      return
    }
    onSubmit(submittableText, imageUrls)
    setText('')
  }

  const estimatePlan: EstimatePlan | null = estimateModel
    ? getEstimatePlan(estimateModel as never, groupValue)
    : null
  const estimateGroups: EstimateGroup[] = estimateModel
    ? getEstimateGroups(estimateModel as never, groupValue)
    : []

  return (
    <div className='grid shrink-0 gap-4 px-1 md:pb-4'>
      <PromptInput
        className='relative'
        accept='image/*'
        multiple
        maxFiles={8}
        groupClassName='bg-background/95 dark:bg-background/80 border-border/70 shadow-[0_18px_60px_-32px_rgba(0,0,0,0.65)] ring-1 ring-foreground/5 rounded-xl overflow-hidden transition-all duration-200 focus-within:border-primary/45 focus-within:ring-primary/15 focus-within:shadow-[0_22px_70px_-34px_rgba(0,0,0,0.75)]'
        onError={(err) => toast.error(err.message)}
        onSubmit={handleSubmit}
      >
        {/* 已附图片缩略图（图生图 / 多图参考）：可移除。 */}
        <PromptInputAttachments>
          {(attachment) => <PromptInputAttachment data={attachment} />}
        </PromptInputAttachments>
        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='min-h-20 px-5 pt-4 pb-3 leading-7 md:min-h-24 md:text-base'
          disabled={disabled || isPreparingImages}
          onChange={(event) => setText(event.target.value)}
          placeholder={t('Ask anything')}
          value={text}
        />

        <PromptInputFooter className='border-border/60 bg-muted/20 dark:bg-muted/10 border-t px-3 py-2.5 backdrop-blur'>
          {estimatePlan && (
            <CostEstimateHint
              plan={estimatePlan}
              groups={estimateGroups}
              promptTokens={1000}
              completionTokens={500}
            />
          )}
          <PlaygroundInputControls
            disabled={disabled}
            groups={groups}
            groupValue={groupValue}
            isGenerating={isGenerating}
            isModelLoading={isModelLoading}
            models={models}
            modelValue={modelValue}
            onGroupChange={onGroupChange}
            onModelChange={onModelChange}
            onStop={onStop}
            text={text}
            tools={
              <PlaygroundInputTools
                config={config}
                disabled={disabled}
                hasMessages={hasMessages}
                onConfigChange={onConfigChange}
                onClearMessages={onClearMessages}
                onParameterEnabledChange={onParameterEnabledChange}
                parameterEnabled={parameterEnabled}
              />
            }
          />
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}
