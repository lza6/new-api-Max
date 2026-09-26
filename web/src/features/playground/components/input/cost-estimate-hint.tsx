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

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'

import {
  estimateModelCostUSD,
  estimateTaskRangeUSD,
  type EstimateGroup,
  type EstimatePlan,
} from '../../lib/cost-estimate'

interface CostEstimateHintProps {
  plan: EstimatePlan
  groups: EstimateGroup[]
  promptTokens: number
  completionTokens: number
}

const TOKEN_PRESETS = [
  { label: '1k / 500', prompt: 1_000, completion: 500 },
  { label: '2k / 1k', prompt: 2_000, completion: 1_000 },
  { label: '4k / 2k', prompt: 4_000, completion: 2_000 },
  { label: '8k / 4k', prompt: 8_000, completion: 4_000 },
]

function formatRange(min: number, max: number): string {
  return `${formatBillingCurrencyFromUSD(min)} – ${formatBillingCurrencyFromUSD(max)}`
}

export function CostEstimateHint({
  plan,
  groups,
  promptTokens,
  completionTokens,
}: CostEstimateHintProps) {
  const { t } = useTranslation()
  const { models } = usePricingData()
  const [open, setOpen] = useState(false)
  const [extraMultiplier, setExtraMultiplier] = useState(1)
  const [prompt, setPrompt] = useState(promptTokens)
  const [completion, setCompletion] = useState(completionTokens)
  const model = models.find((item) => item.model_name === plan.modelName)

  if (!model) {return null}
  if (plan.kind !== 'task' && !model.billing_expr) {return null}

  const multiplier = Math.min(Math.max(Number(extraMultiplier) || 1, 1), 60)

  const rows = groups.map(({ group, ratio }) => {
    if (plan.kind === 'task') {
      const range = estimateTaskRangeUSD(model, [{ group: 'x', ratio }])
      if (range.length === 0) {return { group, range: null }}
      return {
        group,
        range: {
          min: range[0].min * multiplier,
          max: range[0].max * multiplier,
        },
      }
    }
    const single = estimateModelCostUSD(model, ratio, prompt, completion, 1)
    if (single === null) {return { group, range: null }}
    return {
      group,
      range: {
        min: single * multiplier,
        max: single * multiplier,
      },
    }
  })

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className='rounded-md border p-3'
    >
      <CollapsibleTrigger
        render={<Button type='button' variant='outline' size='sm' />}
      >
        {t('Estimated cost range')}
      </CollapsibleTrigger>
      <CollapsibleContent className='mt-3 space-y-3'>
        <p className='text-muted-foreground text-xs'>
          {t('Estimated token usage')}: {prompt.toLocaleString()} →{' '}
          {completion.toLocaleString()}
        </p>
        {plan.kind === 'task' && plan.multiplierFields.length > 0 && (
          <div className='flex items-center gap-2'>
            <Field>
              <FieldLabel htmlFor='estimate-multiplier'>
                {t('Multiplier')}
              </FieldLabel>
              <Input
                id='estimate-multiplier'
                type='number'
                min={1}
                max={60}
                value={extraMultiplier}
                onChange={(event) =>
                  setExtraMultiplier(Number(event.target.value))
                }
                className='w-24'
              />
            </Field>
            <FieldDescription>
              {t('Estimated multiplier for {{fields}}', {
                fields: plan.multiplierFields.join(', '),
              })}
            </FieldDescription>
          </div>
        )}
        {plan.kind === 'token' && (
          <div className='flex flex-wrap gap-1.5'>
            {TOKEN_PRESETS.map((preset) => (
              <Button
                key={preset.label}
                type='button'
                size='xs'
                variant='outline'
                onClick={() => {
                  setPrompt(preset.prompt)
                  setCompletion(preset.completion)
                }}
              >
                {preset.label}
              </Button>
            ))}
          </div>
        )}
        {rows.map(({ group, range }) => (
          <div
            key={group}
            className='flex items-center justify-between gap-2 text-sm'
          >
            <span className='text-muted-foreground'>{group}</span>
            {range ? (
              <span className='font-mono'>
                {range.min === range.max
                  ? formatBillingCurrencyFromUSD(range.min)
                  : formatRange(range.min, range.max)}
              </span>
            ) : (
              <span className='text-muted-foreground'>—</span>
            )}
          </div>
        ))}
        <p className='text-muted-foreground/60 text-xs'>
          {t('Estimate is for reference only, actual billing prevails')}
        </p>
      </CollapsibleContent>
    </Collapsible>
  )
}
