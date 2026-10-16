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
import { AlertTriangle, Loader2, Send, Sparkles } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { handleServerError } from '@/lib/handle-server-error'

import { analyzeTrainingDataConversation } from '../api'

/**
 * 一轮对话。
 *
 * 带 `id` 而非用数组下标做 React key：下标 key 在列表变化时会错配
 * DOM 节点（oxlint 的 no-array-index-key 也会拦）。
 * 这里 id 由单调递增的计数器给出，稳定且唯一。
 */
type ChatTurn = { id: number; role: 'user' | 'assistant'; content: string }

/** 供 API 发送的形态（服务端只认 role/content）。 */
type WireMessage = { role: string; content: string }

/** 单调递增的轮次 id（模块级：同一面板内不会重复）。 */
let turnIdSeq = 0
function nextTurnId(): number {
  turnIdSeq += 1
  return turnIdSeq
}

/**
 * 从模型返回体里取出文本。
 *
 * 走的是 relay 栈，返回体是标准 chat completion；但不同渠道有差异，
 * 这里**容错**读取并返回 null 而不是抛错 —— 让调用方决定如何提示。
 */
function extractAssistantText(payload: unknown): string | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }
  const choices = (payload as { choices?: unknown }).choices
  if (!Array.isArray(choices) || choices.length === 0) {
    return null
  }
  const first = choices[0] as {
    message?: { content?: unknown }
    delta?: { content?: unknown }
  }
  const content = first.message?.content ?? first.delta?.content
  if (typeof content === 'string') {
    return content
  }
  // 有些渠道把 content 返回成 [{type:'text',text:'...'}]。
  if (Array.isArray(content)) {
    const joined = content
      .map((part) =>
        part && typeof part === 'object' && typeof (part as { text?: unknown }).text === 'string'
          ? (part as { text: string }).text
          : ''
      )
      .join('')
    return joined || null
  }
  return null
}

/**
 * 对话深度分析面板（B9）+ 继续追问（B10）。
 *
 * ## 必须在界面上说清楚的事
 *
 * 1. **这是真实调用模型**：会消耗额度，记在**发起分析的管理员**账上。
 *    不提示的话，管理员会以为这只是「看看数据」。
 * 2. **数据边界**（随采集能力演进更新）：
 *    - **请求侧与模型回复都会被采集**，服务端会给正文加角色标注，
 *      模型据此区分发言方 —— 所以「用户原话」可靠，不会被模型回复冒充。
 *    - 但请求侧那一行是上游**拼接后的整段文本**（历史多轮的用户/助手
 *      文本混在一起），**逐轮配对只能启发式还原**，不可断言
 *      「用户第 N 轮说了 X 之后模型回了 Y」。
 *    - 记录有长度上限，超限时服务端会**如实标注截断**。
 */
export function AnalysisPanel(props: { path: string | null }) {
  const { t } = useTranslation()
  const [model, setModel] = useState('')
  const [question, setQuestion] = useState('')
  const [turns, setTurns] = useState<ChatTurn[]>([])
  const [busy, setBusy] = useState(false)

  const targetPath = props.path

  const runAnalysis = async (userQuestion: string | null) => {
    if (!targetPath) {
      return
    }
    setBusy(true)
    try {
      // 首次分析不传 messages（服务端用内置分析提示词）；
      // 追问则把已有对话发回去，让模型接上文。
      // 发送时剥掉本地用的 id，只发服务端认的 role/content。
      const wire: WireMessage[] = turns.map(({ role, content }) => ({
        role,
        content,
      }))
      const messages: WireMessage[] = userQuestion
        ? [...wire, { role: 'user', content: userQuestion }]
        : []

      const res = await analyzeTrainingDataConversation({
        path: targetPath,
        model: model || undefined,
        messages: messages.length > 0 ? messages : undefined,
      })

      if (!res.success) {
        throw new Error(res.message || t('Analysis failed'))
      }

      const text = extractAssistantText(res.data)
      if (!text) {
        // 不静默失败：明确提示，便于排查渠道差异。
        toast.error(t('The model returned no readable text.'))
        return
      }

      const next: ChatTurn[] = userQuestion
        ? [
            ...turns,
            { id: nextTurnId(), role: 'user', content: userQuestion },
            { id: nextTurnId(), role: 'assistant', content: text },
          ]
        : [{ id: nextTurnId(), role: 'assistant', content: text }]
      setTurns(next)
      if (userQuestion) {
        setQuestion('')
      }
    } catch (error) {
      handleServerError(error, t('Analysis failed'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-sm'>
          <Sparkles className='h-4 w-4' />
          {t('Conversation analysis')}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        {/* 费用提示：真实调用模型。 */}
        <div className='border-amber-500/40 bg-amber-500/5 text-amber-700 dark:text-amber-400 flex items-start gap-2 rounded-lg border p-3 text-xs leading-relaxed'>
          <AlertTriangle className='mt-0.5 size-3.5 shrink-0' />
          <span>
            {t(
              'Analysis sends the conversation to a model and consumes quota billed to your admin account. Both the user messages and the model replies are included, each labelled by role, so quoted user statements are reliable. The request side is the gateway-concatenated text of all turns, so per-turn pairing can only be approximated.'
            )}
          </span>
        </div>

        {!targetPath ? (
          <p className='text-muted-foreground text-sm'>
            {t('Pick a file in the browser, then run an analysis.')}
          </p>
        ) : (
          <>
            <div className='text-muted-foreground truncate text-xs'>
              {t('Target')}: <span className='font-mono'>{targetPath}</span>
            </div>

            <div className='flex flex-wrap items-center gap-2'>
              <Input
                className='max-w-56'
                value={model}
                onChange={(e) => setModel(e.target.value)}
                placeholder={t('Model (optional)')}
                aria-label={t('Model (optional)')}
              />
              <Button
                onClick={() => void runAnalysis(null)}
                disabled={busy}
                size='sm'
              >
                {busy && <Loader2 className='mr-1.5 size-3.5 animate-spin' />}
                {t('Start analysis')}
              </Button>
              {turns.length > 0 && (
                <Button
                  variant='ghost'
                  size='sm'
                  disabled={busy}
                  onClick={() => setTurns([])}
                >
                  {t('Clear')}
                </Button>
              )}
            </div>

            {turns.length > 0 && (
              <div className='max-h-[520px] space-y-3 overflow-auto rounded-lg border p-3'>
                {turns.map((turn) => (
                  <div key={turn.id} className='space-y-1'>
                    <p className='text-muted-foreground text-[11px] font-medium uppercase'>
                      {turn.role === 'user' ? t('You') : t('Model')}
                    </p>
                    <p className='text-sm leading-relaxed break-words whitespace-pre-wrap'>
                      {turn.content}
                    </p>
                  </div>
                ))}
              </div>
            )}

            {/* 追问（B10）：把上文一起发回，模型能接着聊。 */}
            <div className='flex items-end gap-2'>
              <Textarea
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                rows={2}
                placeholder={t('Follow-up question…')}
                aria-label={t('Follow-up question')}
                disabled={busy}
              />
              <Button
                size='icon'
                disabled={busy || question.trim() === ''}
                aria-label={t('Send')}
                onClick={() => void runAnalysis(question.trim())}
              >
                <Send className='size-4' />
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
