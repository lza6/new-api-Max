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

import { PlaygroundChat } from './components/chat/playground-chat'
import { AgentPresetSwitcher } from './components/header/agent-preset-switcher'
import { PlaygroundInput } from './components/input/playground-input'
import { PlaygroundSidebar } from './components/sidebar/playground-sidebar'
import { VideoGenerationControls } from './components/video/video-generation-controls'
import { VideoResultPanel } from './components/video/video-result-panel'
import {
  useChatHandler,
  usePlaygroundConversation,
  usePlaygroundOptions,
  usePlaygroundState,
  useSidebarCollapsed,
  useVideoGeneration,
} from './hooks'
import { isVideoModel } from './lib/video/video-generation-utils'
import { UseAsReferenceProvider } from '@/components/use-as-reference-context'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { useProfile } from '@/features/profile/hooks'

export function Playground({ initialModel }: { initialModel?: string }) {
  const { t } = useTranslation()
  const {
    config,
    parameterEnabled,
    messages,
    conversations,
    activeConversationId,
    isLoadingMessages,
    models,
    groups,
    updateMessages,
    setModels,
    setGroups,
    updateConfig,
    updateParameterEnabled,
    clearMessages,
    createNewConversation,
    switchConversation,
    deleteConversation,
  } = usePlaygroundState(initialModel)

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

  const {
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  } = usePlaygroundConversation({
    messages,
    updateMessages,
    sendChat,
  })

  const handleClearMessages = () => {
    handleEditOpenChange(false)
    clearMessages()
  }

  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })

  const { models: pricingModels } = usePricingData()

  // T13 Agent 预设：从用户设置解析（与记忆/技能同一条持久化管线）。
  const { profile, refreshProfile } = useProfile()
  const [activePresetId, setActivePresetId] = useState('')

  // T1 会话侧边栏折叠（持久化到 localStorage）。
  const sidebar = useSidebarCollapsed()

  // T5 「用作参考图」：对话里的图片点按钮后，把它的 URL 交给输入框转成附件
  // （图生图 / 二次改图）。用 state 中转是因为触发方在消息区、消费方在输入框
  // 子树内，两者无共同祖先可直接传 props。
  const [pendingReference, setPendingReference] = useState<string | null>(null)
  const handleUseAsReference = (url: string) => {
    if (isVideoModel(config.model)) {
      // 视频模型没有「参考图」概念，不要静默无事发生。
      toast.error(t('Reference images are only supported by image models.'))
      return
    }
    setPendingReference(url)
  }

  // P1-6 视频生成：仅对视频模型启用，参数（时长/分辨率）随会话保留。
  const video = useVideoGeneration()
  const isVideo = isVideoModel(config.model)
  const [videoSeconds, setVideoSeconds] = useState('')
  const [videoSize, setVideoSize] = useState('')

  // 视频模式下回车不再走对话，而是提交异步视频任务并轮询。
  const handleVideoSubmit = (text: string) => {
    void video.generate({
      config,
      prompt: text,
      seconds: videoSeconds,
      size: videoSize,
    })
  }

  const estimateModel =
    pricingModels.find((item) => item.model_name === config.model) ?? null

  return (
    // T5：生成图走通用 markdown 渲染链，无法直接收到 props；
    // 用 Context 在游乐场边界内声明「可把图片送回输入框」，不污染通用组件。
    <UseAsReferenceProvider value={handleUseAsReference}>
    <div className='flex size-full min-h-0 overflow-hidden'>
      {/* T1 左侧会话侧边栏：新建 / 按时间分组 / 搜索 / 折叠 / 删除。
          原先只有正文上方一个 Select，会话多了无法浏览与定位。 */}
      <PlaygroundSidebar
        conversations={conversations}
        activeConversationId={activeConversationId}
        collapsed={sidebar.collapsed}
        disabled={isGenerating}
        onCreate={createNewConversation}
        onSwitch={switchConversation}
        onDelete={deleteConversation}
        onToggleCollapsed={sidebar.toggle}
      />

      <div className='relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden'>
        {/* 预设工具条：会话导航已移到左侧，这里只留 Agent 预设切换。 */}
        <div className='mx-auto flex w-full max-w-4xl items-center justify-end px-1 pt-2'>
          <AgentPresetSwitcher
            config={config}
            disabled={isGenerating}
            setting={profile?.setting}
            activePresetId={activePresetId}
            onActivePresetChange={setActivePresetId}
            onProfileUpdate={refreshProfile}
            onApplyPreset={(next) => {
              // 逐字段写回，复用既有 updateConfig（会自动落盘到 localStorage）。
              updateConfig('model', next.model)
              updateConfig('group', next.group)
              updateConfig('temperature', next.temperature)
              updateConfig('max_tokens', next.max_tokens)
              updateConfig('reasoning_effort', next.reasoning_effort)
              updateConfig('system_prompt', next.system_prompt)
            }}
          />
        </div>
        {/* Full-width scroll container: scrolling works even over side whitespace */}
        <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
          <PlaygroundChat
            messages={messages}
            isLoadingMessages={isLoadingMessages}
            onRegenerateMessage={handleRegenerateMessage}
            onEditMessage={handleEditMessage}
            onDeleteMessage={handleDeleteMessage}
            onSelectPrompt={handleSendMessage}
            onUseAsReference={handleUseAsReference}
            mediaModel={config.model}
            mediaSize={isVideo ? videoSize : config.size}
            isGenerating={isGenerating}
            editingKey={editingMessageKey}
            onCancelEdit={handleEditOpenChange}
            onSaveEdit={(newContent) => applyEdit(newContent, false)}
            onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
          />
        </div>

        {/* Input area: center content and constrain to the same container width */}
        <div className='mx-auto w-full max-w-4xl space-y-2'>
          {isVideo && (
            <VideoGenerationControls
              seconds={videoSeconds}
              size={videoSize}
              disabled={video.isGenerating}
              onSecondsChange={setVideoSeconds}
              onSizeChange={setVideoSize}
            />
          )}
          <PlaygroundInput
            config={config}
            disabled={isGenerating || video.isGenerating}
            groups={groups}
            groupValue={config.group}
            isGenerating={isGenerating || video.isGenerating}
            isModelLoading={isLoadingModels}
            modelValue={config.model}
            models={models}
            onGroupChange={(value) => updateConfig('group', value)}
            onConfigChange={updateConfig}
            onClearMessages={handleClearMessages}
            onModelChange={(value) => updateConfig('model', value)}
            onParameterEnabledChange={updateParameterEnabled}
            onStop={isVideo ? video.cancel : stopGeneration}
            onSubmit={isVideo ? handleVideoSubmit : handleSendMessage}
            parameterEnabled={parameterEnabled}
            hasMessages={messages.length > 0}
            estimateModel={isVideo ? null : estimateModel}
            pendingReference={pendingReference}
            onReferenceConsumed={() => setPendingReference(null)}
            onReferenceError={() =>
              toast.error(t('Could not load that image as a reference.'))
            }
          />
          {isVideo && (
            <VideoResultPanel
              state={video}
              onReset={video.reset}
              onCancel={video.cancel}
            />
          )}
        </div>
      </div>
    </div>
    </UseAsReferenceProvider>
  )
}
