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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Loader } from '@/components/ai-elements/loader'
import { MessageContent } from '@/components/ai-elements/message'
import { ChatImage } from '@/components/chat-image'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
} from '@/components/ai-elements/reasoning'
import { Response } from '@/components/ai-elements/response'
import { Shimmer } from '@/components/ai-elements/shimmer'
import {
  Source,
  Sources,
  SourcesContent,
  SourcesTrigger,
} from '@/components/ai-elements/sources'
import { cn } from '@/lib/utils'

import { MESSAGE_STATUS } from '../../constants'
import { isMediaModel, mediaSkeletonAspectClass } from '../../lib/message/media-skeleton'
import {
  getCurrentVersion,
  getMessageAlignmentClass,
  getMessageContentState,
  isErrorMessage,
  type MessageAlignment,
} from '../../lib'
import { getMessageContentStyles } from '../../lib/message/message-styles'
import type { Message } from '../../types'
import { MessageError } from './message-error'
import { MessageMetadata } from './message-metadata'

type PlaygroundMessageContentProps = {
  actions: ReactNode
  alignment: MessageAlignment
  errorActions?: ReactNode
  isSourceVisible?: boolean
  message: Message
  versionContent: string
  /**
   * 「用作参考图」回调（T5）：把历史图片送回输入框做二次改图（图生图）。
   * 不传时图片不显示该按钮。
   */
  onUseAsReference?: (url: string) => void
  /** T9：当前模型名。仅在「生成媒体」的模型上显示生成中骨架。 */
  mediaModel?: string
  /** T9：当前图片尺寸（如 1024x1536），决定骨架宽高比。 */
  mediaSize?: string
}

export function PlaygroundMessageContent({
  actions,
  alignment,
  errorActions,
  isSourceVisible = false,
  message,
  versionContent,
  onUseAsReference,
  mediaModel = '',
  mediaSize = '',
}: PlaygroundMessageContentProps) {
  const { t } = useTranslation()
  const {
    displayContent,
    hasReasoning,
    hasSources,
    reasoningContent,
    showLoader,
    showMessageContent,
    sources,
  } = getMessageContentState(message, versionContent)
  const isError = isErrorMessage(message)
  const isMessageFinal =
    message.status !== MESSAGE_STATUS.LOADING &&
    message.status !== MESSAGE_STATUS.STREAMING
  // 用户消息附带的图片（图生图 / 多图参考）：在正文上方展示缩略图。
  const userImages = getCurrentVersion(message).images ?? []

  return (
    <div
      className={cn(
        'flex w-full min-w-0 flex-col',
        getMessageAlignmentClass(alignment)
      )}
    >
      {hasSources && (
        <Sources>
          <SourcesTrigger count={sources.length} />
          <SourcesContent>
            {sources.map((source) => (
              <Source
                href={source.href}
                key={`${source.href}-${source.title}`}
                title={source.title}
              />
            ))}
          </SourcesContent>
        </Sources>
      )}

      {hasReasoning && (
        <Reasoning
          defaultOpen
          duration={message.reasoning?.duration}
          isStreaming={message.isReasoningStreaming}
        >
          <ReasoningTrigger />
          <ReasoningContent>{reasoningContent}</ReasoningContent>
        </Reasoning>
      )}

      {showLoader && (
        <div className='flex items-center gap-2 py-2'>
          <Loader />
          <Shimmer className='text-sm' duration={1}>
            {t('Responding...')}
          </Shimmer>
        </div>
      )}

      {/* 生成中骨架（T9）：生图/生视频比文本慢得多（数秒到数分钟），期间只转圈
          会让用户以为卡住。用骨架**预留成品的形状**，成品到达时不会把下方内容
          整体推下去（CLS），也给出「确实在做事」的视觉反馈。
          比例按用户选的 size 推导，未选时用 1:1（生图默认）。 */}
      {showLoader && isMediaModel(mediaModel) && (
        <Skeleton
          className={cn(
            'mt-2 w-full max-w-sm rounded-lg',
            mediaSkeletonAspectClass(mediaSize)
          )}
        />
      )}

      {isError && (
        <>
          <MessageError message={message} className='mb-2' />
          <MessageMetadata alignment={alignment} message={message} />
          {errorActions}
        </>
      )}

      {!isError && showMessageContent && (
        <>
          {userImages.length > 0 && (
            <div className='mb-2 flex flex-wrap gap-2'>
              {userImages.map((url) => (
                <ChatImage
                  key={`${message.key}-img-${url}`}
                  src={url}
                  alt={t('Attached image')}
                  imgClassName='max-h-40'
                  onUseAsReference={onUseAsReference}
                />
              ))}
            </div>
          )}
          {isSourceVisible ? (
            <CodeBlock
              code={versionContent}
              className='my-0 group-[.is-assistant]:w-full group-[.is-assistant]:max-w-[78ch]'
              collapsedLines={24}
              defaultCollapsed={false}
              language='markdown'
              maxExpandedLines={48}
              showLineNumbers
              showToolbar
              title={t('Raw response')}
            >
              <CodeBlockCopyButton />
            </CodeBlock>
          ) : (
            // 图片-only 消息（无文字）时不渲染空正文。
            displayContent.length > 0 && (
              <MessageContent
                variant='flat'
                className={cn(getMessageContentStyles())}
              >
                <Response final={isMessageFinal}>{displayContent}</Response>
              </MessageContent>
            )
          )}
          <MessageMetadata alignment={alignment} message={message} />
          {actions}
        </>
      )}
    </div>
  )
}
