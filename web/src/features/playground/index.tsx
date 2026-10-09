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

import { PlaygroundChat } from './components/chat/playground-chat'
import { AgentPresetSwitcher } from './components/header/agent-preset-switcher'
import { ConversationSwitcher } from './components/header/conversation-switcher'
import { PlaygroundInput } from './components/input/playground-input'
import {
  useChatHandler,
  usePlaygroundConversation,
  usePlaygroundOptions,
  usePlaygroundState,
} from './hooks'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'
import { useProfile } from '@/features/profile/hooks'

export function Playground({ initialModel }: { initialModel?: string }) {
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

  const estimateModel =
    pricingModels.find((item) => item.model_name === config.model) ?? null

  return (
    <div className='relative flex size-full min-h-0 flex-col overflow-hidden'>
      {/* 多会话工具条（P0-4）：新建 / 切换 / 删除会话。 */}
      <div className='mx-auto flex w-full max-w-4xl items-center justify-between px-1 pt-2'>
        <ConversationSwitcher
          conversations={conversations}
          activeConversationId={activeConversationId}
          disabled={isGenerating}
          onCreate={createNewConversation}
          onSwitch={switchConversation}
          onDelete={deleteConversation}
        />
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
          isGenerating={isGenerating}
          editingKey={editingMessageKey}
          onCancelEdit={handleEditOpenChange}
          onSaveEdit={(newContent) => applyEdit(newContent, false)}
          onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
        />
      </div>

      {/* Input area: center content and constrain to the same container width */}
      <div className='mx-auto w-full max-w-4xl'>
        <PlaygroundInput
          config={config}
          disabled={isGenerating}
          groups={groups}
          groupValue={config.group}
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          modelValue={config.model}
          models={models}
          onGroupChange={(value) => updateConfig('group', value)}
          onConfigChange={updateConfig}
          onClearMessages={handleClearMessages}
          onModelChange={(value) => updateConfig('model', value)}
          onParameterEnabledChange={updateParameterEnabled}
          onStop={stopGeneration}
          onSubmit={handleSendMessage}
          parameterEnabled={parameterEnabled}
          hasMessages={messages.length > 0}
          estimateModel={estimateModel}
        />
      </div>
    </div>
  )
}
