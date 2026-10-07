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
import { useCallback, useEffect, useRef, useState } from 'react'

import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  saveConfig,
  saveParameterEnabled,
  applyMessageStateUpdate,
  createConversation,
  deriveConversationTitle,
  getInitialParameterEnabled,
  getInitialPlaygroundConfig,
  loadActiveConversationId,
  loadConversations,
  saveActiveConversationId,
  saveConversations,
  type Conversation,
  type MessageStateUpdater,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
} from '../types'

const MESSAGE_SAVE_DEBOUNCE_MS = 500

/**
 * Main state management hook for playground
 *
 * `initialModel` (from a `?model=` deep link) seeds the model selection so a
 * link from the model plaza opens the playground already pointed at that model.
 * If the seeded id is not among the site's available models, the existing
 * `getModelFallback` in `usePlaygroundOptions` replaces it once the list loads.
 */
export function usePlaygroundState(initialModel?: string) {
  // Load initial state from localStorage
  const [config, setConfig] = useState<PlaygroundConfig>(() => {
    const base = getInitialPlaygroundConfig()
    return initialModel ? { ...base, model: initialModel } : base
  })

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    getInitialParameterEnabled
  )

  const [messages, setMessages] = useState<Message[]>([])
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeConversationId, setActiveConversationId] = useState<string>('')
  const [isLoadingMessages, setIsLoadingMessages] = useState(true)
  const messagesSaveTimerRef = useRef<number | null>(null)
  const latestMessagesRef = useRef<Message[]>(messages)
  const hasLoadedMessagesRef = useRef(false)
  // 会话集合的实时引用（避免 persistMessages 闭包过期读到旧会话）。
  const conversationsRef = useRef<Conversation[]>([])
  const activeConversationIdRef = useRef<string>('')

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])

  // persistConversations 把当前会话的最新标题/时间/消息写回会话列表并落盘。
  const persistConversations = useCallback(
    (nextConversations: Conversation[]) => {
      conversationsRef.current = nextConversations
      saveConversations(nextConversations)
    },
    []
  )

  // persistMessages 把消息写入「当前会话」并防抖落盘（会话标题随首条用户消息更新）。
  const persistMessages = useCallback(
    (messagesToSave: Message[]) => {
      latestMessagesRef.current = messagesToSave

      if (!hasLoadedMessagesRef.current) {
        return
      }

      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
      }

      messagesSaveTimerRef.current = window.setTimeout(() => {
        messagesSaveTimerRef.current = null
        const activeId = activeConversationIdRef.current
        const updated = conversationsRef.current.map((c) =>
          c.id === activeId
            ? {
                ...c,
                messages: latestMessagesRef.current,
                title:
                  // 有用户消息才更新标题（避免 assistant-only 覆盖为占位）。
                  latestMessagesRef.current.some((m) => m.from === 'user')
                    ? deriveConversationTitle(latestMessagesRef.current)
                    : c.title,
                updatedAt: Date.now(),
              }
            : c
        )
        persistConversations(updated)
        setConversations(updated)
      }, MESSAGE_SAVE_DEBOUNCE_MS)
    },
    [persistConversations]
  )

  useEffect(() => {
    let cancelled = false

    window.setTimeout(() => {
      const loadedConversations = loadConversations()
      const savedActiveId = loadActiveConversationId()
      const active =
        loadedConversations.find((c) => c.id === savedActiveId) ??
        loadedConversations[0]
      if (cancelled) {
        return
      }

      conversationsRef.current = loadedConversations
      activeConversationIdRef.current = active.id
      latestMessagesRef.current = active.messages
      hasLoadedMessagesRef.current = true
      setConversations(loadedConversations)
      setActiveConversationId(active.id)
      setMessages(active.messages)
      setIsLoadingMessages(false)
      saveActiveConversationId(active.id)
    }, 0)

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(
    () => () => {
      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
        persistConversations(conversationsRef.current)
      }
    },
    [persistConversations]
  )

  // Update config with automatic save
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated)
        return updated
      })
    },
    []
  )

  // Update parameter enabled with automatic save
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  // Update messages with automatic save
  const updateMessages = useCallback(
    (updater: MessageStateUpdater) => {
      setMessages((prev) => {
        const newMessages = applyMessageStateUpdate(prev, updater)
        persistMessages(newMessages)
        return newMessages
      })
    },
    [persistMessages]
  )

  // Clear all messages (current conversation). 先冲刷未落盘的消息，避免切换后丢失。
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  // 新建会话：切到新会话（空消息），当前会话已由防抖落盘保留。
  const createNewConversation = useCallback(() => {
    if (messagesSaveTimerRef.current !== null) {
      window.clearTimeout(messagesSaveTimerRef.current)
      messagesSaveTimerRef.current = null
      persistConversations(
        conversationsRef.current.map((c) =>
          c.id === activeConversationIdRef.current
            ? { ...c, messages: latestMessagesRef.current, updatedAt: Date.now() }
            : c
        )
      )
    }
    const fresh = createConversation()
    // 与持久化上限保持一致（保留最近更新的 50 个），避免内存/落盘列表分叉。
    const next = [...conversationsRef.current, fresh]
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, 50)
      .sort((a, b) => a.createdAt - b.createdAt)
    persistConversations(next)
    setConversations(next)
    activeConversationIdRef.current = fresh.id
    latestMessagesRef.current = fresh.messages
    setActiveConversationId(fresh.id)
    setMessages(fresh.messages)
    saveActiveConversationId(fresh.id)
  }, [persistConversations])

  // 切换会话：落盘当前会话，再载入目标会话。
  const switchConversation = useCallback(
    (id: string) => {
      if (id === activeConversationIdRef.current) {
        return
      }
      if (messagesSaveTimerRef.current !== null) {
        window.clearTimeout(messagesSaveTimerRef.current)
        messagesSaveTimerRef.current = null
        persistConversations(
          conversationsRef.current.map((c) =>
            c.id === activeConversationIdRef.current
              ? {
                  ...c,
                  messages: latestMessagesRef.current,
                  updatedAt: Date.now(),
                }
              : c
          )
        )
      }
      const target = conversationsRef.current.find((c) => c.id === id)
      if (!target) {
        return
      }
      activeConversationIdRef.current = id
      latestMessagesRef.current = target.messages
      setActiveConversationId(id)
      setMessages(target.messages)
      saveActiveConversationId(id)
    },
    [persistConversations]
  )

  // 删除会话：删除后若删的是当前会话，切到剩余第一个；全删则新建一个。
  const deleteConversation = useCallback(
    (id: string) => {
      let next = conversationsRef.current.filter((c) => c.id !== id)
      if (next.length === 0) {
        next = [createConversation()]
      }
      persistConversations(next)
      setConversations(next)
      if (id === activeConversationIdRef.current) {
        const target = next[0]
        activeConversationIdRef.current = target.id
        latestMessagesRef.current = target.messages
        setActiveConversationId(target.id)
        setMessages(target.messages)
        saveActiveConversationId(target.id)
      }
    },
    [persistConversations]
  )

  // Reset config to defaults
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
  }, [])

  return {
    // State
    config,
    parameterEnabled,
    messages,
    conversations,
    activeConversationId,
    isLoadingMessages,
    models,
    groups,

    // Setters
    setModels,
    setGroups,

    // Actions
    updateConfig,
    updateParameterEnabled,
    updateMessages,
    clearMessages,
    createNewConversation,
    switchConversation,
    deleteConversation,
    resetConfig,
  }
}
