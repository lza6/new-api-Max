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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

/** `model=quota` 每行一条；`#` 开头为注释。空文本 = 不限制。 */
export const serializeModelLimits = (limits: Record<string, number>): string =>
  Object.keys(limits)
    .sort()
    .map((model) => `${model}=${limits[model]}`)
    .join('\n')

export type ParsedModelLimits =
  | { ok: true; value: Record<string, number> }
  | { ok: false; line: string }

export const parseModelLimits = (text: string): ParsedModelLimits => {
  const value: Record<string, number> = {}
  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue
    const cut = line.lastIndexOf('=')
    if (cut <= 0) return { ok: false, line }
    const model = line.slice(0, cut).trim()
    const limit = Number(line.slice(cut + 1).trim())
    if (!model || !Number.isFinite(limit) || limit < 0) {
      return { ok: false, line }
    }
    value[model] = limit
  }
  return { ok: true, value }
}

/** 每行一个模型名；`#` 开头为注释。空 = 白名单为空（语义缓存不生效）。 */
export const parseModelList = (text: string): string[] =>
  text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'))

const serializeModelList = (models: string[]): string => models.join('\n')

const safeJSON = (text: string): unknown => {
  if (!text.trim()) return null
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

/** 服务端返回的模型预算可能是 JSON 字符串或对象，统一成 `model=quota` 文本。 */
export const toModelLimitsText = (raw: unknown): string => {
  const parsed = typeof raw === 'string' ? safeJSON(raw) : raw
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return ''
  const limits: Record<string, number> = {}
  for (const [model, value] of Object.entries(parsed as Record<string, unknown>)) {
    const limit = Number(value)
    if (model && Number.isFinite(limit) && limit >= 0) limits[model] = limit
  }
  return serializeModelLimits(limits)
}

/** 服务端返回的白名单可能是 JSON 字符串或数组，统一成「每行一个模型」。 */
export const toModelListText = (raw: unknown): string => {
  const parsed = typeof raw === 'string' ? safeJSON(raw) : raw
  if (!Array.isArray(parsed)) return ''
  return serializeModelList(parsed.map((item) => String(item)).filter(Boolean))
}

const createSchema = () =>
  z.object({
    maxPromptChars: z.number().int().min(0).max(10000000),
    // 两个文本域会原样写进 option；加上限避免一次粘贴把整份配置写爆。
    modelLimits: z.string().max(20000),
    semanticModels: z.string().max(20000),
    semanticSimilarity: z.number().min(0).max(1),
  })

type FormValues = z.infer<ReturnType<typeof createSchema>>

export interface PolicyCenterSectionProps {
  defaultValues: {
    maxPromptChars: number
    /** `model=quota` 文本（已从服务端 JSON 反序列化）。 */
    modelLimits: string
    /** 每行一个模型名。 */
    semanticModels: string
    semanticSimilarity: number
  }
}

/**
 * 统一策略中心的可编辑配置 + 语义缓存的准入配置。
 *
 * 这里只放「管理员必须能改、且改错有明确后果」的四项：
 *  - 提示词长度上限与模型单请求预算：策略引擎 enforce 时才真拦截；
 *  - 语义缓存白名单与阈值：错命中会把「另一个问题的答案」返回给用户，
 *    所以白名单默认为空（= 不启用），阈值默认 0.97（保守）。
 *
 * 开关本身（POLICY_ENGINE_MODE / SEMANTIC_RESPONSE_CACHE）在「实验功能」页，
 * 这里不重复。
 */
export function PolicyCenterSection({ defaultValues }: PolicyCenterSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createSchema()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (values: FormValues) => {
    const limits = parseModelLimits(values.modelLimits)
    if (!limits.ok) {
      form.setError('modelLimits', {
        message: t('Invalid line: {{line}}', { line: limits.line }),
      })
      return
    }
    const updates: Array<[string, string | number]> = [
      ['relay.policy_max_prompt_chars', values.maxPromptChars],
      ['relay.policy_model_request_limits', JSON.stringify(limits.value)],
      [
        'response_cache.semantic_allow_models',
        JSON.stringify(parseModelList(values.semanticModels)),
      ],
      ['response_cache.semantic_similarity', values.semanticSimilarity],
    ]
    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value })
    }
  }

  return (
    <SettingsSection title={t('Unified Policy Center')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save policy center'
          />

          <div className='grid gap-4 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='maxPromptChars'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum prompt characters')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseInt(e.target.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Prompts longer than this are blocked when the policy engine is in enforce mode. 0 disables the check.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='semanticSimilarity'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Semantic similarity threshold')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      max={1}
                      step={0.01}
                      {...field}
                      onChange={(e) =>
                        field.onChange(parseFloat(e.target.value) || 0)
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'How close a prompt must be to reuse a cached answer (0-1, default 0.97). Lower values save more money and risk answering a different question.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='modelLimits'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Per-model request budget (model=quota per line)')}
                </FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    className='font-mono text-xs'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'A request whose estimated cost exceeds its model limit is blocked in enforce mode. Empty means no per-model limit.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='semanticModels'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Semantic cache models (one per line)')}
                </FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    className='font-mono text-xs'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Semantic cache only applies to these models. Leave empty to keep it off entirely.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
