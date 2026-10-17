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
import { api } from '@/lib/api'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { analyzeTrainingDataConversation } from './api'

/**
 * 对话分析接口的**响应形态契约**测试。
 *
 * 为什么必须有这一组：这个接口把请求交给 relay 栈，成功时写的是**上游原始
 * chat completion 体**（没有本项目的 `{success,data}` 信封），而参数错误走
 * `common.ApiErrorMsg`（有 `success:false`）。两种形态**同为 HTTP 200**。
 *
 * 曾经前端按信封解包并让调用方判 `res.success` → 成功路径因 `undefined` 判真
 * 而必然抛错 → 分析/报告/追问三条功能全链路不可用。而既有测试**只覆盖错误路径**
 * （付费红线不允许真实调用模型），所以一直没暴露。
 *
 * 这里锁定「原样透传、不做信封解包」—— 一旦有人再把它包成
 * `{success,data}`，解包后 `choices` 会消失，下面的断言立刻失败。
 */

const chatCompletionBody = {
  id: 'chatcmpl-test',
  object: 'chat.completion',
  choices: [
    {
      index: 0,
      message: { role: 'assistant', content: '## 分析报告\n主题：压测调优' },
      finish_reason: 'stop',
    },
  ],
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('analyzeTrainingDataConversation response contract', () => {
  test('成功响应原样透传：choices 在顶层，且不带 success 字段', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: chatCompletionBody,
    } as never)

    const res = await analyzeTrainingDataConversation({ path: 'day/user/model' })

    // 关键断言 1：`choices` 必须在顶层（若被包进 data 就会消失）。
    expect(res.choices).toBeDefined()
    expect(res.choices?.[0]?.message?.content).toContain('分析报告')
    // 关键断言 2：成功响应**没有** `success` 字段 ——
    // 因此调用方只能写 `success === false` 判错，写 `!success` 会把成功当失败。
    expect(res.success).toBeUndefined()
  })

  test('参数错误响应带 success:false 与 message（同为 HTTP 200）', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: false, message: 'no conversation content found' },
    } as never)

    const res = await analyzeTrainingDataConversation({ path: 'empty' })

    expect(res.success).toBe(false)
    expect(res.message).toContain('no conversation content')
    // 错误响应里没有 choices，调用方据此可区分两种形态。
    expect(res.choices).toBeUndefined()
  })

  test('发送的请求体把 path 与可选字段交给后端', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({
      data: chatCompletionBody,
    } as never)

    await analyzeTrainingDataConversation({
      path: 'day/user/model',
      model: 'gpt-4o',
      messages: [{ role: 'user', content: '继续分析' }],
    })

    expect(post).toHaveBeenCalledWith(
      '/api/admin/training-data/analyze',
      expect.objectContaining({
        path: 'day/user/model',
        model: 'gpt-4o',
        messages: [{ role: 'user', content: '继续分析' }],
      })
    )
  })
})
