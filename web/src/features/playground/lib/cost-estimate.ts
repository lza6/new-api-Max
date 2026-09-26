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
import { getConfiguredGroupRatio } from '@/features/pricing/lib/model-helpers'
import { evaluateBillingExpression } from '@/features/pricing/lib/billing-expression/runtime'
import { evaluateTaskUsageExamples } from '@/features/pricing/lib/task-expr'
import type { PricingModel } from '@/features/pricing/types'

const ESTIMATE_MAX_MULTIPLIER = 60
const ESTIMATE_CAP = 10_000
const TOKEN_COST_SCALE = 1_000_000

/** Usage fields that act as billing multipliers in task/token expressions. */
export const ESTIMATE_MULTIPLIER_FIELDS = ['n', 'duration', 'seconds'] as const
export type EstimateMultiplierField = (typeof ESTIMATE_MULTIPLIER_FIELDS)[number]

export function clampEstimateMultiplier(value: number): number {
  if (!Number.isFinite(value) || value <= 0) {return 1}
  return Math.min(value, ESTIMATE_MAX_MULTIPLIER)
}

export function clampEstimateValue(value: number): number {
  if (!Number.isFinite(value) || value < 0) {return 0}
  return Math.min(value, ESTIMATE_CAP)
}

export function isEstimateMultiplierField(field: string): boolean {
  return (ESTIMATE_MULTIPLIER_FIELDS as readonly string[]).includes(field)
}

export type EstimatePlan = {
  modelName: string
  group: string
  kind: 'token' | 'task'
  multiplierFields: string[]
}

export function getEstimatePlan(
  model: PricingModel | undefined,
  group: string
): EstimatePlan | null {
  if (!model) {return null}
  const schema = model.billing_usage_schema ?? {}
  const multiplierFields = Object.keys(schema).filter((field) =>
    isEstimateMultiplierField(field)
  )
  const kind = multiplierFields.length > 0 ? 'task' : 'token'
  return {
    modelName: model.model_name,
    group,
    kind,
    multiplierFields,
  }
}

export type EstimateGroup = {
  group: string
  ratio: number
}

export function getEstimateGroups(
  model: PricingModel | undefined,
  group: string
): EstimateGroup[] {
  const enableGroups = Array.isArray(model?.enable_groups)
    ? model.enable_groups
    : []
  const modelGroupRatio = model?.group_ratio ?? {}
  const allGroups = new Map<string, number>()
  for (const name of [...enableGroups, group]) {
    if (!name || name === 'auto') {continue}
    const ratio = getConfiguredGroupRatio(modelGroupRatio, name)
    allGroups.set(name, ratio)
  }
  return [...allGroups.entries()]
    .sort(([, left], [, right]) => left - right)
    .map(([name, ratio]) => ({ group: name, ratio }))
}

/**
 * Total USD estimate of one token request, applying the group ratio and an
 * optional extra multiplier (e.g. image `n` / task `duration`).
 * Token expressions are unscaled (tokens x unit price); divide by 1M to get
 * USD, matching the billing simulator display. Returns null when the model
 * has no executable billing expression or the evaluation fails.
 */
export function estimateModelCostUSD(
  model: PricingModel,
  groupRatio: number,
  promptTokens: number,
  completionTokens: number,
  extraMultiplier: number
): number | null {
  if (!model.billing_expr) {return null}
  const result = evaluateBillingExpression(model.billing_expr, {
    tokens: {
      p: clampEstimateValue(promptTokens),
      c: clampEstimateValue(completionTokens),
    },
    request: { body: {} },
  })
  if (result.status !== 'success') {return null}
  const multiplier = clampEstimateMultiplier(extraMultiplier)
  if (result.billingUnit === 'token') {
    return Math.max((result.cost / TOKEN_COST_SCALE) * groupRatio * multiplier, 0)
  }
  const fixedPrice = result.fixedPrice ?? result.cost / TOKEN_COST_SCALE
  return Math.max(fixedPrice * groupRatio * multiplier, 0)
}

export type EstimateGroupRange = {
  group: string
  ratio: number
  min: number
  max: number
}

/**
 * Task-model USD range over the configured usage examples, per user group.
 * The examples carry the true usage facts (including n / duration / seconds),
 * so this is the honest range source for task billing.
 */
export function estimateTaskRangeUSD(
  model: PricingModel,
  groups: EstimateGroup[]
): EstimateGroupRange[] {
  const schema = model.billing_usage_schema ?? {}
  const examples = model.billing_usage_examples ?? []
  if (Object.keys(schema).length === 0 || examples.length === 0) {return []}
  const usageRows = evaluateTaskUsageExamples(
    model.billing_expr ?? '',
    schema,
    examples
  )
  if (usageRows.length === 0) {return []}
  const totals = usageRows.map((row) => row.total)
  const minTotal = Math.min(...totals)
  const maxTotal = Math.max(...totals)
  return groups.map(({ group, ratio }) => ({
    group,
    ratio,
    min: minTotal * ratio,
    max: maxTotal * ratio,
  }))
}
